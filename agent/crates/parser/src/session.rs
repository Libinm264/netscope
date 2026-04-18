/// Session manager: receives PacketEvents from the capture layer,
/// feeds them through the TCP reassembler and protocol parsers,
/// and emits complete Flow objects.
use crate::dns::parse_dns;
use crate::http::{looks_like_http_request, looks_like_http_response, parse_request, parse_response};
use crate::http2::{looks_like_h2, H2Session};
use crate::tls::{looks_like_tls, parse_tls};
use capture::tcp_stream::{Direction, TcpReassembler};
use chrono::{DateTime, Utc};
use etherparse::{Icmpv4Type, SlicedPacket, TransportSlice};
use proto::{
    ArpFlow, DnsFlow, Flow, FlowPayload, HttpFlow, IcmpFlow, PacketEvent, Protocol, TcpStats,
};
use std::collections::HashMap;
use tracing::debug;
use uuid::Uuid;

// ── HTTP session state ────────────────────────────────────────────────────────

/// Tracks the partially-assembled HTTP exchange for a TCP connection.
#[derive(Debug, Default)]
struct HttpSession {
    client_buf: Vec<u8>,
    server_buf: Vec<u8>,
    request: Option<proto::HttpRequest>,
    request_time: Option<DateTime<Utc>>,
}

// ── ICMP echo tracking ─────────────────────────────────────────────────────────

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
struct EchoKey {
    src_ip: String,
    id: u16,
    seq: u16,
}

// ── TLS port heuristic ────────────────────────────────────────────────────────

fn is_tls_port(port: u16) -> bool {
    matches!(port, 443 | 8443 | 465 | 587 | 636 | 993 | 995 | 5061 | 8883)
}

// ── SessionManager ────────────────────────────────────────────────────────────

pub struct SessionManager {
    reassembler: TcpReassembler,
    /// key = canonical TcpKey string, value = in-progress HTTP/1.1 session
    http_sessions: HashMap<String, HttpSession>,
    /// key = canonical TcpKey string, value = in-progress HTTP/2 session
    h2_sessions: HashMap<String, H2Session>,
    /// ICMP echo requests waiting for their reply: key → send timestamp
    echo_requests: HashMap<EchoKey, DateTime<Utc>>,
}

impl SessionManager {
    pub fn new() -> Self {
        SessionManager {
            reassembler: TcpReassembler::new(),
            http_sessions: HashMap::new(),
            h2_sessions: HashMap::new(),
            echo_requests: HashMap::new(),
        }
    }

    /// Process a raw PacketEvent and return any completed flows.
    pub fn process(&mut self, event: &PacketEvent) -> Vec<Flow> {
        match event.protocol {
            Protocol::Dns  => self.process_dns(event),
            Protocol::Icmp => self.process_icmp(event),
            Protocol::Arp  => self.process_arp(event),
            Protocol::Tcp | Protocol::Unknown => self.process_tcp(event),
            _ => vec![],
        }
    }

    // ── DNS ───────────────────────────────────────────────────────────────────

    fn process_dns(&self, event: &PacketEvent) -> Vec<Flow> {
        if event.dst_port != Some(53) && event.src_port != Some(53) {
            return vec![];
        }
        if let Some(dns_flow) = self.parse_dns_from_packet(&event.raw) {
            return vec![Flow {
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
                tcp_stats: None,
            }];
        }
        vec![]
    }

    // ── ICMP ──────────────────────────────────────────────────────────────────

    fn process_icmp(&mut self, event: &PacketEvent) -> Vec<Flow> {
        let (icmp_type, icmp_code, echo_id, echo_seq) =
            match extract_icmp_fields(&event.raw) {
                Some(v) => v,
                None => return vec![],
            };

        let (type_str, rtt_ms) = match icmp_type {
            8 => {
                // Echo Request — store and emit immediately
                if let (Some(id), Some(seq)) = (echo_id, echo_seq) {
                    self.echo_requests.insert(
                        EchoKey { src_ip: event.src_ip.clone(), id, seq },
                        event.timestamp,
                    );
                }
                (icmp_type_str(icmp_type, icmp_code), None)
            }
            0 => {
                // Echo Reply — look up matching request for RTT
                let rtt = if let (Some(id), Some(seq)) = (echo_id, echo_seq) {
                    let key = EchoKey { src_ip: event.dst_ip.clone(), id, seq };
                    self.echo_requests.remove(&key).map(|sent| {
                        (event.timestamp - sent)
                            .num_microseconds()
                            .map(|us| us as f64 / 1000.0)
                            .unwrap_or(0.0)
                    })
                } else {
                    None
                };
                (icmp_type_str(icmp_type, icmp_code), rtt)
            }
            _ => (icmp_type_str(icmp_type, icmp_code), None),
        };

        vec![Flow {
            id: new_id(),
            timestamp: event.timestamp,
            src_ip: event.src_ip.clone(),
            dst_ip: event.dst_ip.clone(),
            src_port: 0,
            dst_port: 0,
            protocol: Protocol::Icmp,
            bytes_in: event.length as u64,
            bytes_out: 0,
            payload: Some(FlowPayload::Icmp(IcmpFlow {
                icmp_type,
                icmp_code,
                type_str,
                echo_id,
                echo_seq,
                rtt_ms,
            })),
            tcp_stats: None,
        }]
    }

    // ── ARP ───────────────────────────────────────────────────────────────────

    fn process_arp(&self, event: &PacketEvent) -> Vec<Flow> {
        let arp = match parse_arp_from_packet(&event.raw) {
            Some(a) => a,
            None => return vec![],
        };

        vec![Flow {
            id: new_id(),
            timestamp: event.timestamp,
            src_ip: arp.sender_ip.clone(),
            dst_ip: arp.target_ip.clone(),
            src_port: 0,
            dst_port: 0,
            protocol: Protocol::Arp,
            bytes_in: event.length as u64,
            bytes_out: 0,
            payload: Some(FlowPayload::Arp(arp)),
            tcp_stats: None,
        }]
    }

    // ── TCP (HTTP + TLS) ──────────────────────────────────────────────────────

    fn process_tcp(&mut self, event: &PacketEvent) -> Vec<Flow> {
        let mut flows = Vec::new();
        let tcp_data_list = self.reassembler.process(&event.raw);

        for tcp_data in tcp_data_list {
            let key = format!(
                "{}:{}-{}:{}",
                tcp_data.key.src_ip, tcp_data.key.src_port,
                tcp_data.key.dst_ip, tcp_data.key.dst_port,
            );

            let stats = TcpStats {
                retransmissions: tcp_data.retransmits,
                out_of_order: tcp_data.out_of_order,
            };

            // ── TLS detection ─────────────────────────────────────────────────
            let is_tls = is_tls_port(tcp_data.key.dst_port)
                || is_tls_port(tcp_data.key.src_port)
                || looks_like_tls(&tcp_data.data);

            if is_tls {
                if let Some(tls_hs) = parse_tls(&tcp_data.data) {
                    flows.push(Flow {
                        id: new_id(),
                        timestamp: event.timestamp,
                        src_ip: tcp_data.key.src_ip.clone(),
                        dst_ip: tcp_data.key.dst_ip.clone(),
                        src_port: tcp_data.key.src_port,
                        dst_port: tcp_data.key.dst_port,
                        protocol: Protocol::Tls,
                        bytes_in: tcp_data.data.len() as u64,
                        bytes_out: 0,
                        payload: Some(FlowPayload::Tls(tls_hs)),
                        tcp_stats: Some(stats.clone()),
                    });
                }
                if tcp_data.fin {
                    self.http_sessions.remove(&key);
                }
                continue;
            }

            // ── HTTP/2 detection ──────────────────────────────────────────────
            // Detect the h2c client connection preface (unencrypted HTTP/2).
            // TLS-wrapped HTTP/2 (h2) is handled post-decryption (future work).
            let is_h2 = looks_like_h2(&tcp_data.data)
                || self.h2_sessions.contains_key(&key);

            if is_h2 {
                let session = self.h2_sessions.entry(key.clone()).or_default();
                let h2_flows = match tcp_data.direction {
                    Direction::ClientToServer => session.push_client(&tcp_data.data),
                    Direction::ServerToClient => session.push_server(&tcp_data.data),
                };
                for h2f in h2_flows {
                    let proto = if h2f.grpc_service.is_some() {
                        Protocol::Grpc
                    } else {
                        Protocol::Http2
                    };
                    flows.push(Flow {
                        id: new_id(),
                        timestamp: event.timestamp,
                        src_ip: tcp_data.key.src_ip.clone(),
                        dst_ip: tcp_data.key.dst_ip.clone(),
                        src_port: tcp_data.key.src_port,
                        dst_port: tcp_data.key.dst_port,
                        protocol: proto,
                        bytes_in: tcp_data.data.len() as u64,
                        bytes_out: 0,
                        payload: Some(FlowPayload::Http2(h2f)),
                        tcp_stats: Some(stats.clone()),
                    });
                }
                if tcp_data.fin {
                    self.h2_sessions.remove(&key);
                }
                continue;
            }

            // ── HTTP detection ────────────────────────────────────────────────
            let is_likely_http = tcp_data.key.dst_port == 80
                || tcp_data.key.src_port == 80
                || tcp_data.key.dst_port == 8080
                || tcp_data.key.src_port == 8080
                || tcp_data.key.dst_port == 3000
                || tcp_data.key.src_port == 3000
                || looks_like_http_request(&tcp_data.data)
                || looks_like_http_response(&tcp_data.data);

            if !is_likely_http {
                if tcp_data.fin {
                    self.http_sessions.remove(&key);
                }
                continue;
            }

            let session = self.http_sessions.entry(key.clone()).or_default();

            match tcp_data.direction {
                Direction::ClientToServer => {
                    session.client_buf.extend_from_slice(&tcp_data.data);
                    if session.request.is_none() {
                        match parse_request(&session.client_buf) {
                            Ok(Some((req, consumed))) => {
                                session.request_time = Some(req.timestamp);
                                session.request = Some(req);
                                session.client_buf.drain(..consumed);
                            }
                            Ok(None) => {}
                            Err(e) => {
                                debug!("HTTP request parse error on {}: {}", key, e);
                                session.client_buf.clear();
                            }
                        }
                    }
                }
                Direction::ServerToClient => {
                    session.server_buf.extend_from_slice(&tcp_data.data);
                    if session.request.is_some() {
                        match parse_response(&session.server_buf) {
                            Ok(Some((resp, consumed))) => {
                                let req = session.request.take().unwrap();
                                let request_time = session.request_time.take();
                                let latency_ms = request_time.map(|t| {
                                    (resp.timestamp - t).num_milliseconds().max(0) as u64
                                });

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
                                    payload: Some(FlowPayload::Http(HttpFlow {
                                        request: Some(req),
                                        response: Some(resp),
                                        latency_ms,
                                    })),
                                    tcp_stats: Some(stats.clone()),
                                });

                                session.server_buf.drain(..consumed);
                            }
                            Ok(None) => {}
                            Err(e) => {
                                debug!("HTTP response parse error on {}: {}", key, e);
                                session.server_buf.clear();
                            }
                        }
                    }
                }
            }

            if tcp_data.fin {
                self.http_sessions.remove(&key);
            }
        }

        flows
    }

    // ── Packet field extractors ───────────────────────────────────────────────

    fn parse_dns_from_packet(&self, raw: &[u8]) -> Option<DnsFlow> {
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

// ── ICMP helpers ──────────────────────────────────────────────────────────────

/// Extract (type, code, echo_id, echo_seq) from a raw packet.
fn extract_icmp_fields(raw: &[u8]) -> Option<(u8, u8, Option<u16>, Option<u16>)> {
    let sliced = SlicedPacket::from_ethernet(raw)
        .or_else(|_| SlicedPacket::from_ip(raw))
        .ok()?;

    match sliced.transport {
        Some(TransportSlice::Icmpv4(icmp)) => {
            let (icmp_type, icmp_code, echo_id, echo_seq) = match icmp.icmp_type() {
                Icmpv4Type::EchoRequest(h) => (8u8, 0u8, Some(h.id), Some(h.seq)),
                Icmpv4Type::EchoReply(h)   => (0u8, 0u8, Some(h.id), Some(h.seq)),
                Icmpv4Type::DestinationUnreachable(h) => {
                    (3u8, h.code_u8(), None, None)
                }
                Icmpv4Type::TimeExceeded(_) => (11u8, 0u8, None, None),
                Icmpv4Type::Unknown { type_u8, code_u8, .. } => {
                    (type_u8, code_u8, None, None)
                }
                _ => return None,
            };
            Some((icmp_type, icmp_code, echo_id, echo_seq))
        }
        Some(TransportSlice::Icmpv6(icmp)) => {
            let raw_type = icmp.type_u8();
            let raw_code = icmp.code_u8();
            // ICMPv6 echo = 128 (request) / 129 (reply)
            match raw_type {
                128 | 129 => {
                    // payload bytes 4-5 = id, 6-7 = seq
                    let payload = icmp.payload();
                    let (id, seq) = if payload.len() >= 4 {
                        (
                            Some(u16::from_be_bytes([payload[0], payload[1]])),
                            Some(u16::from_be_bytes([payload[2], payload[3]])),
                        )
                    } else {
                        (None, None)
                    };
                    Some((raw_type, raw_code, id, seq))
                }
                _ => Some((raw_type, raw_code, None, None)),
            }
        }
        _ => None,
    }
}

fn icmp_type_str(icmp_type: u8, icmp_code: u8) -> String {
    match icmp_type {
        0  => "Echo Reply".to_string(),
        3  => match icmp_code {
            0 => "Destination Unreachable (Net)".to_string(),
            1 => "Destination Unreachable (Host)".to_string(),
            2 => "Destination Unreachable (Protocol)".to_string(),
            3 => "Destination Unreachable (Port)".to_string(),
            _ => format!("Destination Unreachable (code {})", icmp_code),
        },
        4  => "Source Quench".to_string(),
        5  => "Redirect".to_string(),
        8  => "Echo Request".to_string(),
        11 => match icmp_code {
            0 => "TTL Exceeded in Transit".to_string(),
            1 => "Fragment Reassembly Time Exceeded".to_string(),
            _ => format!("Time Exceeded (code {})", icmp_code),
        },
        12 => "Parameter Problem".to_string(),
        // ICMPv6
        128 => "Echo Request (v6)".to_string(),
        129 => "Echo Reply (v6)".to_string(),
        133 => "Router Solicitation".to_string(),
        134 => "Router Advertisement".to_string(),
        135 => "Neighbor Solicitation".to_string(),
        136 => "Neighbor Advertisement".to_string(),
        _   => format!("ICMP type {} code {}", icmp_type, icmp_code),
    }
}

// ── ARP helper ────────────────────────────────────────────────────────────────

fn parse_arp_from_packet(raw: &[u8]) -> Option<ArpFlow> {
    // Ethernet header is 14 bytes; ARP starts at offset 14
    if raw.len() < 42 { return None; } // 14 + 28 minimum
    let arp = &raw[14..];
    let operation = u16::from_be_bytes([arp[6], arp[7]]);

    let fmt_mac = |b: &[u8]| {
        format!(
            "{:02x}:{:02x}:{:02x}:{:02x}:{:02x}:{:02x}",
            b[0], b[1], b[2], b[3], b[4], b[5]
        )
    };
    let fmt_ip = |b: &[u8]| format!("{}.{}.{}.{}", b[0], b[1], b[2], b[3]);

    Some(ArpFlow {
        operation: if operation == 1 { "who-has".to_string() } else { "is-at".to_string() },
        sender_mac: fmt_mac(&arp[8..14]),
        sender_ip:  fmt_ip(&arp[14..18]),
        target_mac: fmt_mac(&arp[18..24]),
        target_ip:  fmt_ip(&arp[24..28]),
    })
}

// ── ID generator ─────────────────────────────────────────────────────────────

fn new_id() -> String {
    Uuid::new_v4().to_string()
}
