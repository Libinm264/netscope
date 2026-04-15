/// HTTP/1.1 request and response parser built on top of the `httparse` crate.
///
/// The parser is fed reassembled TCP stream data. It produces `HttpRequest`
/// and `HttpResponse` values once a complete message head (and optionally body)
/// is available.
use chrono::Utc;
use httparse::{Request, Response, EMPTY_HEADER};
use proto::{HttpRequest, HttpResponse};
use thiserror::Error;

const MAX_HEADERS: usize = 64;
const BODY_PREVIEW_BYTES: usize = 512;

#[derive(Error, Debug)]
pub enum HttpParseError {
    #[error("HTTP parse error: {0}")]
    Parse(String),
    #[error("Invalid UTF-8 in HTTP data")]
    Utf8(#[from] std::str::Utf8Error),
}

/// Try to parse an HTTP/1.1 request from a byte buffer.
///
/// Returns `Ok(Some((request, bytes_consumed)))` on success,
/// `Ok(None)` when the buffer is incomplete (need more data),
/// `Err` on a definitive parse error.
pub fn parse_request(buf: &[u8]) -> Result<Option<(HttpRequest, usize)>, HttpParseError> {
    let mut headers = [EMPTY_HEADER; MAX_HEADERS];
    let mut req = Request::new(&mut headers);

    match req.parse(buf) {
        Ok(httparse::Status::Complete(header_len)) => {
            let method = req
                .method
                .ok_or_else(|| HttpParseError::Parse("missing method".into()))?
                .to_string();
            let path = req
                .path
                .ok_or_else(|| HttpParseError::Parse("missing path".into()))?
                .to_string();
            let version = format!("HTTP/1.{}", req.version.unwrap_or(1));

            let parsed_headers: Vec<(String, String)> = req
                .headers
                .iter()
                .filter(|h| !h.name.is_empty())
                .map(|h| {
                    let name = h.name.to_string();
                    let value = String::from_utf8_lossy(h.value).to_string();
                    (name, value)
                })
                .collect();

            let body_bytes = &buf[header_len..];
            let content_length = content_length_from_headers(&parsed_headers);

            // For simplicity in Phase 1: if Content-Length is present, check we have enough.
            // If Transfer-Encoding: chunked, we take what we have.
            let (body_preview, total_consumed) = match content_length {
                Some(cl) if body_bytes.len() < cl => {
                    // Incomplete body — return what we have for preview
                    let preview = body_preview(body_bytes);
                    (preview, header_len + body_bytes.len())
                }
                Some(cl) => {
                    let preview = body_preview(&body_bytes[..cl]);
                    (preview, header_len + cl)
                }
                None => (body_preview(body_bytes), header_len),
            };

            Ok(Some((
                HttpRequest {
                    method,
                    path,
                    version,
                    headers: parsed_headers,
                    body_preview,
                    timestamp: Utc::now(),
                },
                total_consumed,
            )))
        }
        Ok(httparse::Status::Partial) => Ok(None),
        Err(e) => Err(HttpParseError::Parse(e.to_string())),
    }
}

/// Try to parse an HTTP/1.1 response from a byte buffer.
pub fn parse_response(buf: &[u8]) -> Result<Option<(HttpResponse, usize)>, HttpParseError> {
    let mut headers = [EMPTY_HEADER; MAX_HEADERS];
    let mut resp = Response::new(&mut headers);

    match resp.parse(buf) {
        Ok(httparse::Status::Complete(header_len)) => {
            let status_code = resp
                .code
                .ok_or_else(|| HttpParseError::Parse("missing status code".into()))?;
            let status_text = resp.reason.unwrap_or("").to_string();
            let version = format!("HTTP/1.{}", resp.version.unwrap_or(1));

            let parsed_headers: Vec<(String, String)> = resp
                .headers
                .iter()
                .filter(|h| !h.name.is_empty())
                .map(|h| {
                    (
                        h.name.to_string(),
                        String::from_utf8_lossy(h.value).to_string(),
                    )
                })
                .collect();

            let body_bytes = &buf[header_len..];
            let content_length = content_length_from_headers(&parsed_headers);

            let (body_preview, total_consumed) = match content_length {
                Some(cl) if body_bytes.len() < cl => {
                    (body_preview(body_bytes), header_len + body_bytes.len())
                }
                Some(cl) => (body_preview(&body_bytes[..cl]), header_len + cl),
                None => (body_preview(body_bytes), header_len),
            };

            Ok(Some((
                HttpResponse {
                    status_code,
                    status_text,
                    version,
                    headers: parsed_headers,
                    body_preview,
                    timestamp: Utc::now(),
                },
                total_consumed,
            )))
        }
        Ok(httparse::Status::Partial) => Ok(None),
        Err(e) => Err(HttpParseError::Parse(e.to_string())),
    }
}

/// Detect whether a buffer looks like an HTTP request (for protocol sniffing).
pub fn looks_like_http_request(buf: &[u8]) -> bool {
    const METHODS: &[&[u8]] = &[
        b"GET ", b"POST ", b"PUT ", b"DELETE ", b"HEAD ",
        b"OPTIONS ", b"PATCH ", b"CONNECT ", b"TRACE ",
    ];
    METHODS.iter().any(|m| buf.starts_with(m))
}

/// Detect whether a buffer looks like an HTTP response.
pub fn looks_like_http_response(buf: &[u8]) -> bool {
    buf.starts_with(b"HTTP/1.")
}

fn content_length_from_headers(headers: &[(String, String)]) -> Option<usize> {
    headers
        .iter()
        .find(|(k, _)| k.eq_ignore_ascii_case("content-length"))
        .and_then(|(_, v)| v.trim().parse().ok())
}

fn body_preview(body: &[u8]) -> Option<String> {
    if body.is_empty() {
        return None;
    }
    let preview = &body[..body.len().min(BODY_PREVIEW_BYTES)];
    // Attempt UTF-8; fall back to lossy
    Some(String::from_utf8_lossy(preview).to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parse_simple_get_request() {
        let raw = b"GET /api/users HTTP/1.1\r\nHost: example.com\r\nUser-Agent: test/1.0\r\n\r\n";
        let result = parse_request(raw).unwrap();
        let (req, consumed) = result.unwrap();
        assert_eq!(req.method, "GET");
        assert_eq!(req.path, "/api/users");
        assert_eq!(req.version, "HTTP/1.1");
        assert_eq!(consumed, raw.len());
        assert!(req
            .headers
            .iter()
            .any(|(k, v)| k == "Host" && v == "example.com"));
    }

    #[test]
    fn parse_post_request_with_body() {
        let body = b"{\"user\":\"alice\"}";
        let raw = format!(
            "POST /api/users HTTP/1.1\r\nHost: api.example.com\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            std::str::from_utf8(body).unwrap()
        );
        let result = parse_request(raw.as_bytes()).unwrap();
        let (req, consumed) = result.unwrap();
        assert_eq!(req.method, "POST");
        assert_eq!(req.path, "/api/users");
        assert_eq!(consumed, raw.len());
        assert!(req.body_preview.is_some());
        assert!(req.body_preview.unwrap().contains("alice"));
    }

    #[test]
    fn parse_200_response() {
        let body = b"Hello, World!";
        let raw = format!(
            "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            std::str::from_utf8(body).unwrap()
        );
        let result = parse_response(raw.as_bytes()).unwrap();
        let (resp, _) = result.unwrap();
        assert_eq!(resp.status_code, 200);
        assert_eq!(resp.status_text, "OK");
        assert!(resp.body_preview.unwrap().contains("Hello"));
    }

    #[test]
    fn parse_404_response() {
        let raw = b"HTTP/1.1 404 Not Found\r\nContent-Length: 0\r\n\r\n";
        let result = parse_response(raw).unwrap();
        let (resp, _) = result.unwrap();
        assert_eq!(resp.status_code, 404);
        assert_eq!(resp.status_text, "Not Found");
    }

    #[test]
    fn incomplete_request_returns_none() {
        let raw = b"GET /partial HTTP/1.1\r\nHost: ex";
        let result = parse_request(raw).unwrap();
        assert!(result.is_none());
    }

    #[test]
    fn looks_like_http_request_detection() {
        assert!(looks_like_http_request(b"GET / HTTP/1.1\r\n"));
        assert!(looks_like_http_request(b"POST /api HTTP/1.1\r\n"));
        assert!(!looks_like_http_request(b"\x16\x03\x01")); // TLS ClientHello
        assert!(!looks_like_http_request(b"HTTP/1.1 200 OK"));
    }

    #[test]
    fn parse_request_multiple_headers() {
        let raw = b"GET /search?q=test HTTP/1.1\r\n\
                   Host: www.google.com\r\n\
                   Accept: text/html,application/xhtml+xml\r\n\
                   Accept-Language: en-US,en;q=0.9\r\n\
                   Connection: keep-alive\r\n\
                   \r\n";
        let result = parse_request(raw).unwrap().unwrap();
        let (req, _) = result;
        assert_eq!(req.method, "GET");
        assert_eq!(req.path, "/search?q=test");
        assert_eq!(req.headers.len(), 4);
    }

    #[test]
    fn parse_response_with_large_body_truncates_preview() {
        let body = "x".repeat(1024);
        let raw = format!(
            "HTTP/1.1 200 OK\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        );
        let (resp, _) = parse_response(raw.as_bytes()).unwrap().unwrap();
        // Body preview should be capped at BODY_PREVIEW_BYTES
        assert!(resp.body_preview.unwrap().len() <= BODY_PREVIEW_BYTES);
    }
}
