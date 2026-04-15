/// TCP stream reassembler.
///
/// Tracks active TCP connections by 4-tuple and reassembles byte streams
/// from individual packets. Handles out-of-order segments.
use etherparse::{InternetSlice, SlicedPacket, TransportSlice};
use std::collections::HashMap;
use tracing::debug;

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct TcpKey {
    pub src_ip: String,
    pub dst_ip: String,
    pub src_port: u16,
    pub dst_port: u16,
}

impl TcpKey {
    pub fn reversed(&self) -> TcpKey {
        TcpKey {
            src_ip: self.dst_ip.clone(),
            dst_ip: self.src_ip.clone(),
            src_port: self.dst_port,
            dst_port: self.src_port,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Direction {
    ClientToServer,
    ServerToClient,
}

pub struct TcpData {
    pub key: TcpKey,
    pub direction: Direction,
    pub data: Vec<u8>,
    pub fin: bool,
}

#[derive(Debug)]
struct TcpStream {
    next_seq: u32,
    pending: HashMap<u32, Vec<u8>>,
    buffer: Vec<u8>,
    finished: bool,
}

impl TcpStream {
    fn new(isn: u32) -> Self {
        TcpStream {
            next_seq: isn,
            pending: HashMap::new(),
            buffer: Vec::new(),
            finished: false,
        }
    }

    /// Returns true if new contiguous bytes were added to the buffer.
    fn push(&mut self, seq: u32, payload: &[u8], fin: bool) -> bool {
        if fin {
            self.finished = true;
        }
        if payload.is_empty() {
            return false;
        }

        let seq_delta = seq.wrapping_sub(self.next_seq) as i32;

        if seq_delta == 0 {
            self.buffer.extend_from_slice(payload);
            self.next_seq = self.next_seq.wrapping_add(payload.len() as u32);

            // Drain pending out-of-order segments that now fit
            loop {
                if let Some(data) = self.pending.remove(&self.next_seq) {
                    self.next_seq = self.next_seq.wrapping_add(data.len() as u32);
                    self.buffer.extend_from_slice(&data);
                } else {
                    break;
                }
            }
            true
        } else if seq_delta > 0 {
            self.pending.insert(seq, payload.to_vec());
            false
        } else {
            debug!("Dropping retransmitted segment seq={} next_seq={}", seq, self.next_seq);
            false
        }
    }

    fn drain(&mut self) -> Vec<u8> {
        std::mem::take(&mut self.buffer)
    }
}

struct Connection {
    client: TcpStream,
    server: TcpStream,
}

pub struct TcpReassembler {
    /// Keyed by the client-side TcpKey (the side that sent SYN)
    connections: HashMap<TcpKey, Connection>,
}

impl TcpReassembler {
    pub fn new() -> Self {
        TcpReassembler {
            connections: HashMap::new(),
        }
    }

    /// Process a raw Ethernet/IP packet. Returns assembled data segments.
    pub fn process(&mut self, raw: &[u8]) -> Vec<TcpData> {
        let sliced = match SlicedPacket::from_ethernet(raw)
            .or_else(|_| SlicedPacket::from_ip(raw))
        {
            Ok(s) => s,
            Err(_) => return vec![],
        };

        // etherparse 0.15: single-field enum variants
        let (src_ip, dst_ip): (String, String) = match &sliced.net {
            Some(InternetSlice::Ipv4(ipv4)) => (
                ipv4.header().source_addr().to_string(),
                ipv4.header().destination_addr().to_string(),
            ),
            Some(InternetSlice::Ipv6(ipv6)) => (
                ipv6.header().source_addr().to_string(),
                ipv6.header().destination_addr().to_string(),
            ),
            None => return vec![],
        };

        // Extract all TCP fields while the borrow is live
        let (src_port, dst_port, seq, payload, syn, fin, rst) = match &sliced.transport {
            Some(TransportSlice::Tcp(tcp)) => (
                tcp.source_port(),
                tcp.destination_port(),
                tcp.sequence_number(),
                tcp.payload().to_vec(),
                tcp.syn(),
                tcp.fin(),
                tcp.rst(),
            ),
            _ => return vec![],
        };

        let pkt_key = TcpKey { src_ip: src_ip.clone(), dst_ip: dst_ip.clone(), src_port, dst_port };

        // Determine canonical (client) key and direction
        let (canon_key, direction) = if self.connections.contains_key(&pkt_key) {
            (pkt_key.clone(), Direction::ClientToServer)
        } else if self.connections.contains_key(&pkt_key.reversed()) {
            (pkt_key.reversed(), Direction::ServerToClient)
        } else if syn {
            (pkt_key.clone(), Direction::ClientToServer)
        } else {
            return vec![];
        };

        if rst {
            self.connections.remove(&canon_key);
            return vec![];
        }

        // SYN from client: create connection
        if syn && direction == Direction::ClientToServer {
            let isn = seq.wrapping_add(1);
            self.connections.insert(
                canon_key.clone(),
                Connection {
                    client: TcpStream::new(isn),
                    server: TcpStream::new(0),
                },
            );
            return vec![];
        }

        let conn = match self.connections.get_mut(&canon_key) {
            Some(c) => c,
            None => return vec![],
        };

        // SYN-ACK from server: initialise server stream
        if syn && direction == Direction::ServerToClient {
            conn.server = TcpStream::new(seq.wrapping_add(1));
            return vec![];
        }

        let new_data = match direction {
            Direction::ClientToServer => conn.client.push(seq, &payload, fin),
            Direction::ServerToClient => conn.server.push(seq, &payload, fin),
        };

        let mut results = vec![];

        if new_data {
            let data = match direction {
                Direction::ClientToServer => conn.client.drain(),
                Direction::ServerToClient => conn.server.drain(),
            };
            if !data.is_empty() {
                results.push(TcpData { key: canon_key.clone(), direction, data, fin });
            }
        }

        if fin && conn.client.finished && conn.server.finished {
            self.connections.remove(&canon_key);
        }

        results
    }

    pub fn connection_count(&self) -> usize {
        self.connections.len()
    }
}

impl Default for TcpReassembler {
    fn default() -> Self {
        Self::new()
    }
}
