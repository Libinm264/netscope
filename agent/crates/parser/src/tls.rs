/// TLS handshake and alert parser.
///
/// Parses TLS 1.0 – 1.3 records from reassembled TCP payloads.
/// Decodes ClientHello (cipher suites, SNI), ServerHello (chosen cipher,
/// negotiated version via extensions), Certificate (CN, SANs, expiry),
/// and Alert (level + description) messages.
///
/// No private-key decryption is performed; only the plaintext handshake
/// messages are parsed.
use chrono::Utc;
use proto::TlsHandshake;
use tracing::debug;

// ── TLS record/handshake type constants ────────────────────────────────────────

const RT_CHANGE_CIPHER_SPEC: u8 = 0x14;
const RT_ALERT: u8 = 0x15;
const RT_HANDSHAKE: u8 = 0x16;

const HS_CLIENT_HELLO: u8 = 0x01;
const HS_SERVER_HELLO: u8 = 0x02;
const HS_CERTIFICATE: u8 = 0x0B;
const HS_FINISHED: u8 = 0x14;

const EXT_SNI: u16 = 0x0000;
const EXT_SUPPORTED_VERSIONS: u16 = 0x002b;

// ── Public API ─────────────────────────────────────────────────────────────────

/// Returns true if the buffer looks like the start of a TLS record.
pub fn looks_like_tls(buf: &[u8]) -> bool {
    if buf.len() < 5 {
        return false;
    }
    // ContentType must be one of the known TLS record types
    matches!(buf[0], RT_CHANGE_CIPHER_SPEC | RT_ALERT | RT_HANDSHAKE | 0x17)
        // Legacy version field: 0x03xx
        && buf[1] == 0x03
        && buf[2] <= 0x04
}

/// Attempt to parse a TLS record from a reassembled TCP buffer.
/// Returns Some(TlsHandshake) for the first interesting record found.
pub fn parse_tls(buf: &[u8]) -> Option<TlsHandshake> {
    if buf.len() < 5 {
        return None;
    }

    let record_type = buf[0];
    let version = tls_version_str(buf[1], buf[2]);
    let record_len = u16::from_be_bytes([buf[3], buf[4]]) as usize;

    if buf.len() < 5 + record_len {
        return None; // incomplete record
    }

    let record_body = &buf[5..5 + record_len];

    match record_type {
        RT_ALERT => parse_alert(record_body, &version),

        RT_CHANGE_CIPHER_SPEC => Some(TlsHandshake {
            record_type: "ChangeCipherSpec".to_string(),
            version,
            ..empty_handshake()
        }),

        RT_HANDSHAKE => {
            if record_body.len() < 4 {
                return None;
            }
            let hs_type = record_body[0];
            let hs_len = u24_be(&record_body[1..4]) as usize;
            if record_body.len() < 4 + hs_len {
                return None;
            }
            let hs_body = &record_body[4..4 + hs_len];

            match hs_type {
                HS_CLIENT_HELLO => parse_client_hello(hs_body, &version),
                HS_SERVER_HELLO => parse_server_hello(hs_body, &version),
                HS_CERTIFICATE  => parse_certificate(hs_body, &version),
                HS_FINISHED     => Some(TlsHandshake {
                    record_type: "Finished".to_string(),
                    version,
                    ..empty_handshake()
                }),
                _ => {
                    debug!("TLS: unknown handshake type 0x{:02x}", hs_type);
                    None
                }
            }
        }

        _ => None,
    }
}

// ── ClientHello ────────────────────────────────────────────────────────────────

fn parse_client_hello(body: &[u8], version: &str) -> Option<TlsHandshake> {
    // legacy_version(2) + random(32) = 34 bytes before session_id
    if body.len() < 34 { return None; }

    let mut off = 34; // skip legacy_version + random

    // session_id
    let sid_len = *body.get(off)? as usize;
    off += 1 + sid_len;

    // cipher_suites
    if off + 2 > body.len() { return None; }
    let cs_len = u16::from_be_bytes([body[off], body[off + 1]]) as usize;
    off += 2;
    if off + cs_len > body.len() { return None; }

    let mut cipher_suites = Vec::new();
    let mut has_weak_cipher = false;
    let mut i = 0;
    while i + 1 < cs_len {
        let suite = u16::from_be_bytes([body[off + i], body[off + i + 1]]);
        let name = cipher_suite_name(suite);
        if is_weak_cipher(suite) {
            has_weak_cipher = true;
        }
        // Skip SCSV pseudo-cipher-suites
        if suite != 0x00FF && suite != 0x5600 {
            cipher_suites.push(name);
        }
        i += 2;
    }
    off += cs_len;

    // compression_methods
    if off >= body.len() { return None; }
    let comp_len = body[off] as usize;
    off += 1 + comp_len;

    // extensions (optional in old clients)
    let mut sni: Option<String> = None;
    if off + 2 <= body.len() {
        let ext_total = u16::from_be_bytes([body[off], body[off + 1]]) as usize;
        off += 2;
        let ext_end = off + ext_total;

        while off + 4 <= ext_end.min(body.len()) {
            let ext_type = u16::from_be_bytes([body[off], body[off + 1]]);
            let ext_len  = u16::from_be_bytes([body[off + 2], body[off + 3]]) as usize;
            off += 4;

            if off + ext_len > body.len() { break; }
            let ext_data = &body[off..off + ext_len];
            off += ext_len;

            if ext_type == EXT_SNI {
                sni = parse_sni_extension(ext_data);
            }
        }
    }

    Some(TlsHandshake {
        record_type: "ClientHello".to_string(),
        version: version.to_string(),
        sni,
        cipher_suites,
        has_weak_cipher,
        ..empty_handshake()
    })
}

// ── ServerHello ────────────────────────────────────────────────────────────────

fn parse_server_hello(body: &[u8], version: &str) -> Option<TlsHandshake> {
    if body.len() < 34 { return None; }

    let mut off = 34; // skip legacy_version + random

    // session_id
    let sid_len = *body.get(off)? as usize;
    off += 1 + sid_len;

    if off + 3 > body.len() { return None; }
    let suite = u16::from_be_bytes([body[off], body[off + 1]]);
    let chosen_cipher = Some(cipher_suite_name(suite));
    off += 2; // skip cipher
    off += 1; // skip compression_method

    // Parse extensions for Supported Versions (TLS 1.3 detection)
    let mut negotiated_version: Option<String> = None;
    if off + 2 <= body.len() {
        let ext_total = u16::from_be_bytes([body[off], body[off + 1]]) as usize;
        off += 2;
        let ext_end = off + ext_total;

        while off + 4 <= ext_end.min(body.len()) {
            let ext_type = u16::from_be_bytes([body[off], body[off + 1]]);
            let ext_len  = u16::from_be_bytes([body[off + 2], body[off + 3]]) as usize;
            off += 4;
            if off + ext_len > body.len() { break; }
            let ext_data = &body[off..off + ext_len];
            off += ext_len;

            if ext_type == EXT_SUPPORTED_VERSIONS && ext_len == 2 {
                // Server sends exactly one selected version
                negotiated_version = Some(tls_version_str(ext_data[0], ext_data[1]));
            }
        }
    }

    let weak = is_weak_cipher(suite);
    Some(TlsHandshake {
        record_type: "ServerHello".to_string(),
        version: version.to_string(),
        chosen_cipher,
        negotiated_version,
        has_weak_cipher: weak,
        ..empty_handshake()
    })
}

// ── Certificate ────────────────────────────────────────────────────────────────

fn parse_certificate(body: &[u8], version: &str) -> Option<TlsHandshake> {
    // certificate_list length: 3-byte big-endian
    if body.len() < 3 { return None; }
    let list_len = u24_be(&body[0..3]) as usize;
    if body.len() < 3 + list_len { return None; }

    let cert_area = &body[3..3 + list_len];
    if cert_area.len() < 3 { return None; }

    // First certificate
    let cert_len = u24_be(&cert_area[0..3]) as usize;
    if cert_area.len() < 3 + cert_len { return None; }
    let cert_der = &cert_area[3..3 + cert_len];

    let (cert_cn, cert_sans, cert_expiry, cert_expired, cert_issuer) =
        parse_cert_der(cert_der).unwrap_or_default();

    Some(TlsHandshake {
        record_type: "Certificate".to_string(),
        version: version.to_string(),
        cert_cn,
        cert_sans,
        cert_expiry,
        cert_expired,
        cert_issuer,
        ..empty_handshake()
    })
}

/// Parse a DER-encoded X.509 certificate using x509-parser.
fn parse_cert_der(
    der: &[u8],
) -> Option<(Option<String>, Vec<String>, Option<String>, bool, Option<String>)> {
    use x509_parser::prelude::*;

    let (_, cert) = X509Certificate::from_der(der).ok()?;

    let cn = cert
        .subject()
        .iter_common_name()
        .next()
        .and_then(|a| a.as_str().ok())
        .map(String::from);

    let issuer = cert
        .issuer()
        .iter_common_name()
        .next()
        .and_then(|a| a.as_str().ok())
        .map(String::from);

    let not_after_ts = cert.validity().not_after.timestamp();
    let expired = not_after_ts < Utc::now().timestamp();
    let expiry = chrono::DateTime::from_timestamp(not_after_ts, 0)
        .map(|dt: chrono::DateTime<Utc>| dt.format("%Y-%m-%d").to_string());

    let sans: Vec<String> = cert
        .subject_alternative_name()
        .ok()
        .flatten()
        .map(|ext| {
            ext.value
                .general_names
                .iter()
                .filter_map(|n| match n {
                    GeneralName::DNSName(s) => Some(s.to_string()),
                    GeneralName::IPAddress(b) if b.len() == 4 => {
                        Some(format!("{}.{}.{}.{}", b[0], b[1], b[2], b[3]))
                    }
                    _ => None,
                })
                .collect()
        })
        .unwrap_or_default();

    Some((cn, sans, expiry, expired, issuer))
}

// ── Alert ─────────────────────────────────────────────────────────────────────

fn parse_alert(body: &[u8], version: &str) -> Option<TlsHandshake> {
    if body.len() < 2 { return None; }

    let level = match body[0] {
        1 => "warning".to_string(),
        2 => "fatal".to_string(),
        n => format!("level-{}", n),
    };
    let description = alert_description(body[1]);

    Some(TlsHandshake {
        record_type: "Alert".to_string(),
        version: version.to_string(),
        alert_level: Some(level),
        alert_description: Some(description),
        ..empty_handshake()
    })
}

// ── Helpers ────────────────────────────────────────────────────────────────────

fn empty_handshake() -> TlsHandshake {
    TlsHandshake {
        record_type: String::new(),
        version: String::new(),
        sni: None,
        cipher_suites: vec![],
        has_weak_cipher: false,
        chosen_cipher: None,
        negotiated_version: None,
        cert_cn: None,
        cert_sans: vec![],
        cert_expiry: None,
        cert_expired: false,
        cert_issuer: None,
        alert_level: None,
        alert_description: None,
    }
}

fn u24_be(b: &[u8]) -> u32 {
    (b[0] as u32) << 16 | (b[1] as u32) << 8 | b[2] as u32
}

fn tls_version_str(major: u8, minor: u8) -> String {
    match (major, minor) {
        (0x03, 0x00) => "SSL 3.0".to_string(),
        (0x03, 0x01) => "TLS 1.0".to_string(),
        (0x03, 0x02) => "TLS 1.1".to_string(),
        (0x03, 0x03) => "TLS 1.2".to_string(),
        (0x03, 0x04) => "TLS 1.3".to_string(),
        _            => format!("TLS {}.{}", major, minor),
    }
}

fn parse_sni_extension(data: &[u8]) -> Option<String> {
    // SNI extension body:
    //   server_name_list_length: 2
    //   name_type: 1  (0 = host_name)
    //   name_length: 2
    //   name: name_length bytes
    if data.len() < 5 { return None; }
    let list_len = u16::from_be_bytes([data[0], data[1]]) as usize;
    if data.len() < 2 + list_len { return None; }

    let name_type = data[2];
    if name_type != 0 { return None; } // only host_name supported

    let name_len = u16::from_be_bytes([data[3], data[4]]) as usize;
    if data.len() < 5 + name_len { return None; }

    std::str::from_utf8(&data[5..5 + name_len])
        .ok()
        .map(String::from)
}

fn alert_description(code: u8) -> String {
    match code {
        0   => "close_notify",
        10  => "unexpected_message",
        20  => "bad_record_mac",
        40  => "handshake_failure",
        42  => "bad_certificate",
        44  => "certificate_revoked",
        45  => "certificate_expired",
        46  => "certificate_unknown",
        47  => "illegal_parameter",
        48  => "unknown_ca",
        49  => "access_denied",
        50  => "decode_error",
        51  => "decrypt_error",
        70  => "protocol_version",
        71  => "insufficient_security",
        80  => "internal_error",
        90  => "user_canceled",
        100 => "no_renegotiation",
        110 => "unsupported_extension",
        112 => "unrecognized_name",
        113 => "bad_certificate_status_response",
        115 => "unknown_psk_identity",
        116 => "certificate_required",
        _   => "unknown",
    }
    .to_string()
}

// ── Cipher suite names ────────────────────────────────────────────────────────

fn cipher_suite_name(id: u16) -> String {
    match id {
        // TLS 1.3
        0x1301 => "TLS_AES_128_GCM_SHA256",
        0x1302 => "TLS_AES_256_GCM_SHA384",
        0x1303 => "TLS_CHACHA20_POLY1305_SHA256",
        // ECDHE-ECDSA
        0xC02B => "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256",
        0xC02C => "TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384",
        0xCCA9 => "TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256",
        0xC009 => "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
        0xC00A => "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
        0xC023 => "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA256",
        0xC024 => "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA384",
        // ECDHE-RSA
        0xC02F => "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
        0xC030 => "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384",
        0xCCA8 => "TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
        0xC013 => "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
        0xC014 => "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
        0xC027 => "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA256",
        0xC028 => "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA384",
        // DHE-RSA
        0xCCAA => "TLS_DHE_RSA_WITH_CHACHA20_POLY1305_SHA256",
        0x0033 => "TLS_DHE_RSA_WITH_AES_128_CBC_SHA",
        0x0039 => "TLS_DHE_RSA_WITH_AES_256_CBC_SHA",
        0x0067 => "TLS_DHE_RSA_WITH_AES_128_CBC_SHA256",
        0x006B => "TLS_DHE_RSA_WITH_AES_256_CBC_SHA256",
        0x009E => "TLS_DHE_RSA_WITH_AES_128_GCM_SHA256",
        0x009F => "TLS_DHE_RSA_WITH_AES_256_GCM_SHA384",
        // RSA (no forward secrecy)
        0x009C => "TLS_RSA_WITH_AES_128_GCM_SHA256",
        0x009D => "TLS_RSA_WITH_AES_256_GCM_SHA384",
        0x002F => "TLS_RSA_WITH_AES_128_CBC_SHA",
        0x0035 => "TLS_RSA_WITH_AES_256_CBC_SHA",
        0x003C => "TLS_RSA_WITH_AES_128_CBC_SHA256",
        0x003D => "TLS_RSA_WITH_AES_256_CBC_SHA256",
        // Weak / deprecated
        0x000A => "TLS_RSA_WITH_3DES_EDE_CBC_SHA",
        0x0005 => "TLS_RSA_WITH_RC4_128_SHA",
        0x0004 => "TLS_RSA_WITH_RC4_128_MD5",
        _      => return format!("0x{:04X}", id),
    }
    .to_string()
}

fn is_weak_cipher(id: u16) -> bool {
    matches!(id,
        0x000A | // 3DES
        0x0005 | // RC4-SHA
        0x0004 | // RC4-MD5
        0x0002 | // NULL-SHA
        0x0001 | // NULL-MD5
        // RSA key exchange (no forward secrecy) is a soft warning in modern standards
        0x002F | 0x0035 | 0x003C | 0x003D | 0x009C | 0x009D
    )
}
