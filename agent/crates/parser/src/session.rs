/// Session manager: receives PacketEvents from the capture layer,
/// feeds them through the TCP reassembler and protocol parsers,
/// and emits complete Flow objects.
use crate::dns::parse_dns;
use crate::http::{looks_like_http_request, looks_like_http_response, parse_request, parse_response};
use capture::tcp_stream::{Direction, TcpReassembler};
use proto::{DnsFlow, Flow, FlowPayload, HttpFlow, PacketEvent, Protocol};
use std::collections::HashMap;
use tracing::debug;

/// Tracks the partially-assembled HTTP exchange for a TCP connection.
#[derive(Debug, Default)]
struct HttpSession {
    client_buf: Vec<u8>,
    server_buf: Vec<u8>,
    request: Option<proto::HttpRequest>,
    request_time: Option<chrono::DateTime<chrono::Utc>>,
}

pub struct SessionManager {
    reassembler: TcpReassembler,
    /// key = canonical TcpKey string, value = in-progress HTTP session
    http_sessions: HashMap<String, HttpSession>,
}

impl SessionManager {
    pub fn new() -> Self {
        SessionManager {
            reassembler: TcpReassembler::new(),
            http_sessions: HashMap::new(),
        }
    }

    /// Process a raw PacketEvent and return any completed flows.
    pub fn process(&mut self, event: &PacketEvent) -> Vec<Flow> {
        let mut flows = Vec::new();

        // Handle DNS (UDP port 53)
        if matches!(event.protocol, Protocol::Udp)
            && (event.dst_port == Some(53) || event.src_port == Some(53))
        {
            // DNS payload is past the IP+UDP headers; use the raw field
            // The reassembler doesn't handle UDP; extract payload manually
            if let Some(dns_flow) = self.parse_dns_from_packet(&event.raw) {
                flows.push(Flow {
                    id: new_id(),
                    timestamp: event.timestamp,
                    src_ip: event.src_ip.clone(),
                    dst_ip: event.dst_ip.clone(),
                    src_port: event.src_port.unwrap_or(0),
                    dst_port: event.dst_port.unwrap_or(0),
                    protocol: Protocol::Dns,
                    bytes_in: event.length as u64,
                    bytes_out: 0,
                    payload: Some(FlowPayload::Dns(dns_flow)),
                });
            }
            return flows;
        }

        // Feed TCP packets to the reassembler
        let tcp_data_list = self.reassembler.process(&event.raw);

        for tcp_data in tcp_data_list {
            let key = format!(
                "{}:{}-{}:{}",
                tcp_data.key.src_ip,
                tcp_data.key.src_port,
                tcp_data.key.dst_ip,
                tcp_data.key.dst_port
            );

            // Determine if this looks like HTTP traffic (port heuristic or content sniff)
            let is_likely_http = tcp_data.key.dst_port == 80
                || tcp_data.key.src_port == 80
                || tcp_data.key.dst_port == 8080
                || tcp_data.key.src_port == 8080
                || tcp_data.key.dst_port == 3000
                || tcp_data.key.src_port == 3000
                || looks_like_http_request(&tcp_data.data)
                || looks_like_http_response(&tcp_data.data);

            if !is_likely_http {
                continue;
            }

            let session = self.http_sessions.entry(key.clone()).or_default();

            match tcp_data.direction {
                Direction::ClientToServer => {
                    session.client_buf.extend_from_slice(&tcp_data.data);

                    // Try to parse a request
                    if session.request.is_none() {
                        match parse_request(&session.client_buf) {
                            Ok(Some((req, consumed))) => {
                                session.request_time = Some(req.timestamp);
                                session.request = Some(req);
                                session.client_buf.drain(..consumed);
                            }
                            Ok(None) => {} // need more data
                            Err(e) => {
                                debug!("HTTP request parse error on {}: {}", key, e);
                                session.client_buf.clear();
                            }
                        }
                    }
                }
                Direction::ServerToClient => {
                    session.server_buf.extend_from_slice(&tcp_data.data);

                    // Only try to parse response if we have a request
                    if session.request.is_some() {
                        match parse_response(&session.server_buf) {
                            Ok(Some((resp, consumed))) => {
                                let req = session.request.take().unwrap();
                                let request_time = session.request_time.take();
                                let latency_ms = request_time.map(|t| {
                                    (resp.timestamp - t).num_milliseconds().max(0) as u64
                                });

                                let http_flow = HttpFlow {
                                    request: Some(req),
                                    response: Some(resp),
                                    latency_ms,
                                };

                                flows.push(Flow {
                                    id: new_id(),
                                    timestamp: event.timestamp,
                                    src_ip: tcp_data.key.src_ip.clone(),
                                    dst_ip: tcp_data.key.dst_ip.clone(),
                                    src_port: tcp_data.key.src_port,
                                    dst_port: tcp_data.key.dst_port,
                                    protocol: Protocol::Http,
                                    bytes_in: tcp_data.data.len() as u64,
                                    bytes_out: 0,
                                    payload: Some(FlowPayload::Http(http_flow)),
                                });

                                session.server_buf.drain(..consumed);
                            }
                            Ok(None) => {} // need more data
                            Err(e) => {
                                debug!("HTTP response parse error on {}: {}", key, e);
                                session.server_buf.clear();
                            }
                        }
                    }
                }
            }

            // Clean up session if TCP connection finished
            if tcp_data.fin {
                self.http_sessions.remove(&key);
            }
        }

        flows
    }

    fn parse_dns_from_packet(&self, raw: &[u8]) -> Option<DnsFlow> {
        use etherparse::{SlicedPacket, TransportSlice};

        let sliced = SlicedPacket::from_ethernet(raw)
            .or_else(|_| SlicedPacket::from_ip(raw))
            .ok()?;

        match &sliced.transport {
            Some(TransportSlice::Udp(udp)) => parse_dns(udp.payload()),
            _ => None,
        }
    }
}

impl Default for SessionManager {
    fn default() -> Self {
        Self::new()
    }
}

fn new_id() -> String {
    // Simple incrementing ID for Phase 1; Phase 4 will use proper UUIDs
    use std::sync::atomic::{AtomicU64, Ordering};
    static COUNTER: AtomicU64 = AtomicU64::new(1);
    COUNTER.fetch_add(1, Ordering::Relaxed).to_string()
}
