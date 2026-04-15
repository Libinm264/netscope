/// Minimal DNS parser for UDP packets.
/// Parses the DNS wire format to extract query name, type, and answers.
use proto::{DnsAnswer, DnsFlow};

pub fn parse_dns(payload: &[u8]) -> Option<DnsFlow> {
    if payload.len() < 12 {
        return None;
    }

    let txid = u16::from_be_bytes([payload[0], payload[1]]);
    let flags = u16::from_be_bytes([payload[2], payload[3]]);
    let qr = (flags >> 15) & 1; // 0 = query, 1 = response
    let rcode = flags & 0xF;
    let qdcount = u16::from_be_bytes([payload[4], payload[5]]) as usize;
    let ancount = u16::from_be_bytes([payload[6], payload[7]]) as usize;

    let mut offset = 12;

    // Parse first question
    let (query_name, name_end) = read_name(payload, offset)?;
    offset = name_end;

    if offset + 4 > payload.len() {
        return None;
    }
    let qtype = u16::from_be_bytes([payload[offset], payload[offset + 1]]);
    offset += 4; // skip qtype + qclass

    let query_type = qtype_to_str(qtype).to_string();
    let is_response = qr == 1;

    let rcode_str = if is_response {
        Some(rcode_to_str(rcode).to_string())
    } else {
        None
    };

    // Parse answers (only if response)
    let mut answers = Vec::new();
    if is_response {
        for _ in 0..ancount.min(16) {
            if offset >= payload.len() {
                break;
            }
            if let Some((answer, new_offset)) = parse_answer(payload, offset) {
                answers.push(answer);
                offset = new_offset;
            } else {
                break;
            }
        }
    }

    // Only skip further questions if qdcount > 1 (rare)
    let _ = qdcount; // used implicitly above

    Some(DnsFlow {
        transaction_id: txid,
        query_name,
        query_type,
        is_response,
        answers,
        rcode: rcode_str,
    })
}

fn parse_answer(payload: &[u8], offset: usize) -> Option<(DnsAnswer, usize)> {
    let (name, mut pos) = read_name(payload, offset)?;

    if pos + 10 > payload.len() {
        return None;
    }

    let rtype = u16::from_be_bytes([payload[pos], payload[pos + 1]]);
    pos += 2;
    let _rclass = u16::from_be_bytes([payload[pos], payload[pos + 1]]);
    pos += 2;
    let ttl = u32::from_be_bytes([payload[pos], payload[pos + 1], payload[pos + 2], payload[pos + 3]]);
    pos += 4;
    let rdlength = u16::from_be_bytes([payload[pos], payload[pos + 1]]) as usize;
    pos += 2;

    if pos + rdlength > payload.len() {
        return None;
    }

    let rdata = &payload[pos..pos + rdlength];
    let data = match rtype {
        1 if rdlength == 4 => {
            // A record
            format!("{}.{}.{}.{}", rdata[0], rdata[1], rdata[2], rdata[3])
        }
        28 if rdlength == 16 => {
            // AAAA record
            let parts: Vec<String> = rdata
                .chunks(2)
                .map(|b| format!("{:x}", u16::from_be_bytes([b[0], b[1]])))
                .collect();
            parts.join(":")
        }
        5 | 2 => {
            // CNAME or NS
            read_name(payload, pos).map(|(n, _)| n).unwrap_or_default()
        }
        _ => format!("<{} bytes>", rdlength),
    };

    Some((
        DnsAnswer {
            name,
            record_type: qtype_to_str(rtype).to_string(),
            ttl,
            data,
        },
        pos + rdlength,
    ))
}

/// Read a DNS name at `offset` (handles pointer compression).
fn read_name(payload: &[u8], offset: usize) -> Option<(String, usize)> {
    let mut labels = Vec::new();
    let mut pos = offset;
    let mut end_pos = None;
    let mut jumps = 0;

    loop {
        if pos >= payload.len() {
            return None;
        }

        let len = payload[pos] as usize;

        if len == 0 {
            // End of name
            if end_pos.is_none() {
                end_pos = Some(pos + 1);
            }
            break;
        } else if (len & 0xC0) == 0xC0 {
            // Pointer
            if pos + 1 >= payload.len() {
                return None;
            }
            let ptr = (((len & 0x3F) as usize) << 8) | payload[pos + 1] as usize;
            if end_pos.is_none() {
                end_pos = Some(pos + 2);
            }
            pos = ptr;
            jumps += 1;
            if jumps > 10 {
                return None; // Loop protection
            }
        } else {
            pos += 1;
            if pos + len > payload.len() {
                return None;
            }
            let label = std::str::from_utf8(&payload[pos..pos + len]).ok()?;
            labels.push(label.to_string());
            pos += len;
        }
    }

    Some((labels.join("."), end_pos.unwrap_or(pos + 1)))
}

fn qtype_to_str(qtype: u16) -> &'static str {
    match qtype {
        1 => "A",
        2 => "NS",
        5 => "CNAME",
        6 => "SOA",
        12 => "PTR",
        15 => "MX",
        16 => "TXT",
        28 => "AAAA",
        33 => "SRV",
        255 => "ANY",
        _ => "UNKNOWN",
    }
}

fn rcode_to_str(rcode: u16) -> &'static str {
    match rcode {
        0 => "NOERROR",
        1 => "FORMERR",
        2 => "SERVFAIL",
        3 => "NXDOMAIN",
        4 => "NOTIMP",
        5 => "REFUSED",
        _ => "UNKNOWN",
    }
}
