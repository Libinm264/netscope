/// HTTP/2 frame parser + HPACK header decoder.
///
/// This module recognises the HTTP/2 client connection preface, walks the
/// binary frame stream, decodes HEADERS frames with HPACK, and emits
/// `Http2Flow` payloads with gRPC service/method extraction.
///
/// # Limitations (v1)
/// - Handles only a single stream per TCP segment (most common case).
/// - CONTINUATION frames are collected but not yet reassembled.
/// - DATA frame bodies are not captured (content preview is a Phase 10 item).
/// - Only plaintext HTTP/2 (h2c) is handled here; TLS-upgraded HTTP/2 goes
///   through the TLS parser first and arrives here post-decryption (future).
use chrono::Utc;
use hpack::Decoder;
use proto::{Http2Flow, Http2Request, Http2Response};

// ── HTTP/2 magic ──────────────────────────────────────────────────────────────

/// The 24-byte client connection preface that starts every h2c connection.
const H2_PREFACE: &[u8] = b"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n";

/// Minimum length of the preface we check before investing in full parsing.
const H2_PREFACE_MIN: usize = 6; // "PRI * "

// ── Frame types we care about ─────────────────────────────────────────────────

const FRAME_DATA:         u8 = 0x0;
const FRAME_HEADERS:      u8 = 0x1;
const FRAME_RST_STREAM:   u8 = 0x3;
const FRAME_SETTINGS:     u8 = 0x4;
const FRAME_GOAWAY:       u8 = 0x7;
const FRAME_CONTINUATION: u8 = 0x9;

// ── Frame flags ───────────────────────────────────────────────────────────────

const FLAG_END_HEADERS: u8 = 0x4;
const FLAG_PADDED:      u8 = 0x8;
const FLAG_PRIORITY:    u8 = 0x20;

// ── Public API ────────────────────────────────────────────────────────────────

/// Returns true when the buffer starts with (or could be the start of) the
/// HTTP/2 client connection preface.  Used by the session manager before
/// allocating a full decoder.
pub fn looks_like_h2(data: &[u8]) -> bool {
    let n = data.len().min(H2_PREFACE.len());
    n >= H2_PREFACE_MIN && data[..n] == H2_PREFACE[..n]
}

/// Walk `data` and attempt to decode the first HEADERS frame found.
/// Returns an `Http2Flow` on success or `None` if the data is unrecognisable
/// or contains only non-HEADERS frames (SETTINGS, WINDOW_UPDATE, etc.).
pub fn parse_h2(data: &[u8]) -> Option<Http2Flow> {
    // Skip past the connection preface if present.
    let start = if data.starts_with(H2_PREFACE) {
        H2_PREFACE.len()
    } else {
        0
    };

    let mut pos = start;
    let mut decoder = Decoder::new();

    // Accumulate headers across CONTINUATION frames.
    let mut hpack_buf: Vec<u8> = Vec::new();
    let mut stream_id: u32 = 0;
    let mut is_request = false;

    while pos + 9 <= data.len() {
        let frame = match parse_frame_header(&data[pos..]) {
            Some(f) => f,
            None => break,
        };

        let payload_start = pos + 9;
        let payload_end   = payload_start + frame.length;
        if payload_end > data.len() {
            break; // partial frame — need more data
        }
        let payload = &data[payload_start..payload_end];

        match frame.frame_type {
            FRAME_HEADERS => {
                stream_id  = frame.stream_id;
                is_request = stream_id % 2 == 1; // odd = client-initiated

                let hpack_data = strip_padding_and_priority(payload, frame.flags);
                hpack_buf.extend_from_slice(&hpack_data);

                if frame.flags & FLAG_END_HEADERS != 0 {
                    let headers = decode_hpack(&mut decoder, &hpack_buf);
                    let flow = build_flow(stream_id, &headers, is_request);
                    hpack_buf.clear();
                    return flow;
                }
            }
            FRAME_CONTINUATION => {
                hpack_buf.extend_from_slice(payload);
                if frame.flags & FLAG_END_HEADERS != 0 {
                    let headers = decode_hpack(&mut decoder, &hpack_buf);
                    let flow = build_flow(stream_id, &headers, is_request);
                    hpack_buf.clear();
                    return flow;
                }
            }
            FRAME_SETTINGS | FRAME_DATA | FRAME_RST_STREAM | FRAME_GOAWAY => {
                // skip — keep walking
            }
            _ => {}
        }

        pos = payload_end;
    }

    None
}

// ── Frame header ─────────────────────────────────────────────────────────────

#[derive(Debug)]
struct FrameHeader {
    length:     usize,
    frame_type: u8,
    flags:      u8,
    stream_id:  u32,
}

fn parse_frame_header(buf: &[u8]) -> Option<FrameHeader> {
    if buf.len() < 9 {
        return None;
    }
    let length     = ((buf[0] as usize) << 16) | ((buf[1] as usize) << 8) | (buf[2] as usize);
    let frame_type = buf[3];
    let flags      = buf[4];
    let stream_id  = u32::from_be_bytes([buf[5] & 0x7f, buf[6], buf[7], buf[8]]);
    Some(FrameHeader { length, frame_type, flags, stream_id })
}

// ── HPACK helpers ─────────────────────────────────────────────────────────────

/// Remove optional PADDED and PRIORITY prefix bytes from a HEADERS payload
/// before handing it to the HPACK decoder.
fn strip_padding_and_priority(payload: &[u8], flags: u8) -> Vec<u8> {
    let mut offset = 0usize;

    // PADDED: first byte is pad length
    let pad_len = if flags & FLAG_PADDED != 0 {
        if payload.is_empty() { return vec![]; }
        let p = payload[0] as usize;
        offset += 1;
        p
    } else {
        0
    };

    // PRIORITY: 4 bytes stream dependency + 1 byte weight
    if flags & FLAG_PRIORITY != 0 {
        offset += 5;
    }

    if offset >= payload.len() { return vec![]; }
    let end = payload.len().saturating_sub(pad_len);
    if offset >= end { return vec![]; }
    payload[offset..end].to_vec()
}

fn decode_hpack(decoder: &mut Decoder, data: &[u8]) -> Vec<(String, String)> {
    let mut out = Vec::new();
    let _ = decoder.decode_with_cb(data, |name, value| {
        let k = String::from_utf8_lossy(&name).into_owned();
        let v = String::from_utf8_lossy(&value).into_owned();
        out.push((k, v));
    });
    out
}

// ── Flow construction ─────────────────────────────────────────────────────────

fn build_flow(
    stream_id: u32,
    headers: &[(String, String)],
    is_request: bool,
) -> Option<Http2Flow> {
    let now = Utc::now();

    if is_request {
        let mut method    = String::new();
        let mut path      = String::new();
        let mut authority = String::new();
        let mut scheme    = String::new();

        for (k, v) in headers {
            match k.as_str() {
                ":method"    => method    = v.clone(),
                ":path"      => path      = v.clone(),
                ":authority" => authority = v.clone(),
                ":scheme"    => scheme    = v.clone(),
                _ => {}
            }
        }

        if method.is_empty() && path.is_empty() {
            return None; // not a request HEADERS frame
        }

        let (grpc_service, grpc_method) = extract_grpc(&path, headers);

        Some(Http2Flow {
            stream_id,
            request: Some(Http2Request {
                method,
                path,
                authority,
                scheme,
                headers: headers.to_vec(),
                timestamp: now,
            }),
            response: None,
            latency_ms: None,
            grpc_service,
            grpc_method,
            grpc_status: None,
        })
    } else {
        // Server response HEADERS frame
        let status_code = headers.iter()
            .find(|(k, _)| k == ":status")
            .and_then(|(_, v)| v.parse::<u16>().ok())
            .unwrap_or(0);

        let grpc_status = headers.iter()
            .find(|(k, _)| k == "grpc-status")
            .and_then(|(_, v)| v.parse::<u32>().ok());

        Some(Http2Flow {
            stream_id,
            request: None,
            response: Some(Http2Response {
                status_code,
                headers: headers.to_vec(),
                timestamp: now,
            }),
            latency_ms: None,
            grpc_service: None,
            grpc_method: None,
            grpc_status,
        })
    }
}

/// Extract (service, method) from a gRPC `:path` of the form
/// `/package.Service/Method`.  Returns (None, None) for non-gRPC paths or
/// when the `content-type` header is not `application/grpc*`.
fn extract_grpc(
    path: &str,
    headers: &[(String, String)],
) -> (Option<String>, Option<String>) {
    let is_grpc = headers.iter().any(|(k, v)| {
        k.eq_ignore_ascii_case("content-type")
            && v.starts_with("application/grpc")
    });
    if !is_grpc {
        return (None, None);
    }

    // path format: /[package.]Service/Method
    let trimmed = path.trim_start_matches('/');
    let mut parts = trimmed.splitn(2, '/');
    let service = parts.next().map(str::to_owned);
    let method  = parts.next().map(str::to_owned);
    (service, method)
}

// ── Session state ─────────────────────────────────────────────────────────────

/// Per-TCP-connection state for an in-progress HTTP/2 session.
/// Held in `SessionManager::h2_sessions`.
#[derive(Debug, Default)]
pub struct H2Session {
    pub client_buf: Vec<u8>,
    pub server_buf: Vec<u8>,
    /// Pending request flows keyed by stream ID, waiting for their response.
    pub pending: std::collections::HashMap<u32, (Http2Flow, chrono::DateTime<Utc>)>,
}

impl H2Session {
    /// Feed client-direction bytes; returns any new request flows.
    pub fn push_client(&mut self, data: &[u8]) -> Vec<Http2Flow> {
        self.client_buf.extend_from_slice(data);
        self.drain_flows(&self.client_buf.clone(), true)
    }

    /// Feed server-direction bytes; returns any completed (matched) flows.
    pub fn push_server(&mut self, data: &[u8]) -> Vec<Http2Flow> {
        self.server_buf.extend_from_slice(data);
        self.drain_flows(&self.server_buf.clone(), false)
    }

    fn drain_flows(&mut self, buf: &[u8], is_client: bool) -> Vec<Http2Flow> {
        let mut out = Vec::new();
        let mut decoder = Decoder::new();
        let start = if buf.starts_with(H2_PREFACE) { H2_PREFACE.len() } else { 0 };
        let mut pos = start;

        while pos + 9 <= buf.len() {
            let frame = match parse_frame_header(&buf[pos..]) {
                Some(f) => f,
                None => break,
            };
            let payload_end = pos + 9 + frame.length;
            if payload_end > buf.len() { break; }

            if frame.frame_type == FRAME_HEADERS {
                let payload   = &buf[pos + 9..payload_end];
                let hpack_raw = strip_padding_and_priority(payload, frame.flags);
                if frame.flags & FLAG_END_HEADERS != 0 {
                    let headers = decode_hpack(&mut decoder, &hpack_raw);
                    let stream  = frame.stream_id;

                    if is_client {
                        if let Some(mut flow) = build_flow(stream, &headers, true) {
                            let (svc, mth) = extract_grpc(&flow.request.as_ref().map(|r| r.path.clone()).unwrap_or_default(), &headers);
                            flow.grpc_service = svc;
                            flow.grpc_method  = mth;
                            self.pending.insert(stream, (flow, Utc::now()));
                        }
                    } else if let Some(mut resp_flow) = build_flow(stream, &headers, false) {
                        if let Some((mut req_flow, req_time)) = self.pending.remove(&stream) {
                            let latency = (Utc::now() - req_time).num_milliseconds().max(0) as u64;
                            req_flow.response   = resp_flow.response.take();
                            req_flow.latency_ms = Some(latency);
                            // Carry grpc-status from trailer
                            if resp_flow.grpc_status.is_some() {
                                req_flow.grpc_status = resp_flow.grpc_status;
                            }
                            out.push(req_flow);
                        } else {
                            out.push(resp_flow);
                        }
                    }
                }
            }

            pos = payload_end;
        }

        // Truncate consumed bytes to bound memory use (keep only last 64 KiB).
        if is_client && pos > 0 {
            let keep = self.client_buf.len().saturating_sub(pos).max(65536);
            let drain = self.client_buf.len().saturating_sub(keep);
            self.client_buf.drain(..drain);
        } else if !is_client && pos > 0 {
            let keep = self.server_buf.len().saturating_sub(pos).max(65536);
            let drain = self.server_buf.len().saturating_sub(keep);
            self.server_buf.drain(..drain);
        }

        out
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_looks_like_h2() {
        assert!(looks_like_h2(b"PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n"));
        assert!(looks_like_h2(b"PRI * "));
        assert!(!looks_like_h2(b"GET / HTTP/1.1\r\n"));
        assert!(!looks_like_h2(b"PO"));
    }

    #[test]
    fn test_parse_frame_header() {
        // 9-byte frame header: length=5, type=4 (SETTINGS), flags=0, stream_id=0
        let bytes = [0x00, 0x00, 0x05, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00];
        let frame = parse_frame_header(&bytes).unwrap();
        assert_eq!(frame.length,     5);
        assert_eq!(frame.frame_type, FRAME_SETTINGS);
        assert_eq!(frame.stream_id,  0);
    }

    #[test]
    fn test_grpc_extraction() {
        let headers = vec![
            (":method".to_string(),       "POST".to_string()),
            (":path".to_string(),         "/grpc.health.v1.Health/Check".to_string()),
            (":scheme".to_string(),       "http".to_string()),
            ("content-type".to_string(),  "application/grpc".to_string()),
        ];
        let (svc, mth) = extract_grpc("/grpc.health.v1.Health/Check", &headers);
        assert_eq!(svc, Some("grpc.health.v1.Health".to_string()));
        assert_eq!(mth, Some("Check".to_string()));
    }

    #[test]
    fn test_non_grpc_path() {
        let headers = vec![
            (":method".to_string(),      "POST".to_string()),
            (":path".to_string(),        "/api/v1/flows".to_string()),
            ("content-type".to_string(), "application/json".to_string()),
        ];
        let (svc, mth) = extract_grpc("/api/v1/flows", &headers);
        assert_eq!(svc, None);
        assert_eq!(mth, None);
    }
}
