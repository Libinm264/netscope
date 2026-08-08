/// PostgreSQL wire-protocol (v3) parser.
///
/// Decodes the StartupMessage (for username/database attribution) and the
/// simple query protocol (`Q` / `C` / `E` messages) to surface query text,
/// command tags, row counts, and errors — without needing to instrument the
/// database itself. See `SPEC.md` §4.2.3 for the original scope this
/// implements (N3 in `REMAINING_WORK.md`).
///
/// # Limitations (v1)
/// - Only the simple query protocol is decoded. The extended query protocol
///   (Parse/Bind/Describe/Execute, used by most connection-pooled ORM
///   drivers) is recognised well enough to be skipped cleanly, but its query
///   text is not yet extracted — a natural v2 follow-up.
/// - `PasswordMessage` ('p') payloads are never parsed. This is deliberate:
///   credentials must never leave the agent process (see the crate's
///   `masking` module for the equivalent HTTP-side policy).
/// - One pending query is tracked per connection at a time, matching how the
///   simple query protocol is actually used (send query, wait for
///   CommandComplete/ErrorResponse, repeat).
use proto::PostgresFlow;
use std::collections::HashMap;

// ── Wire constants ────────────────────────────────────────────────────────────

/// Startup packet's protocol version field for protocol 3.0 (0x00030000).
const PROTOCOL_V3: u32 = 196_608;
/// Magic "protocol version" sent instead of a real StartupMessage to request TLS.
const SSL_REQUEST_CODE: u32 = 80_877_103;
/// Magic "protocol version" sent to request GSSAPI encryption (rare, same 1-byte reply shape as SSL).
const GSSAPI_REQUEST_CODE: u32 = 80_877_102;

/// Frontend (client → server) simple query message.
const FE_QUERY: u8 = b'Q';
/// Backend (server → client) command completion.
const BE_COMMAND_COMPLETE: u8 = b'C';
/// Backend (server → client) error response.
const BE_ERROR_RESPONSE: u8 = b'E';

/// The default PostgreSQL TCP port.
pub const DEFAULT_PORT: u16 = 5432;

/// Returns true when `port` is the standard PostgreSQL port. Used by the
/// session manager to decide whether a TCP connection should be routed
/// through this parser (PostgreSQL has no reliable magic byte signature at
/// the start of a stream the way TLS/HTTP2 do, so port is the heuristic).
pub fn is_postgres_port(port: u16) -> bool {
    port == DEFAULT_PORT
}

// ── Generic typed-message framer (shared by frontend + backend messages) ─────

/// Read one length-prefixed PostgreSQL protocol message: a 1-byte type tag
/// followed by a 4-byte big-endian length (which includes itself, but not
/// the type byte). Returns `(type, payload, total_bytes_consumed)`.
fn read_message(buf: &[u8]) -> Option<(u8, &[u8], usize)> {
    if buf.len() < 5 {
        return None;
    }
    let msg_type = buf[0];
    let len = u32::from_be_bytes([buf[1], buf[2], buf[3], buf[4]]) as usize;
    if len < 4 {
        return None; // malformed — length must at least cover itself
    }
    let total = 1 + len;
    if buf.len() < total {
        return None; // incomplete — wait for more data
    }
    Some((msg_type, &buf[5..total], total))
}

/// Read a null-terminated C string from the start of `payload`.
fn parse_cstring(payload: &[u8]) -> Option<String> {
    let end = payload.iter().position(|&b| b == 0)?;
    Some(String::from_utf8_lossy(&payload[..end]).into_owned())
}

// ── StartupMessage ────────────────────────────────────────────────────────────

/// Outcome of inspecting the next client-direction message before the
/// session has completed its startup handshake.
enum StartupStep {
    /// An SSLRequest or GSSAPIRequest was consumed; the server will reply
    /// with a single 'S'/'N' byte (not a normal framed message).
    NegotiationRequest { consumed: usize },
    /// A real StartupMessage was parsed.
    Startup { user: Option<String>, database: Option<String>, consumed: usize },
    /// Not enough data yet to decide.
    Incomplete,
    /// Doesn't look like a startup message at all — give up on attribution
    /// for this connection and fall through to normal message framing.
    NotStartup,
}

/// Inspect the next client-direction bytes, which — at the start of a
/// connection — are either a StartupMessage or an SSLRequest/GSSAPIRequest
/// (both of which lack the usual 1-byte type tag).
fn read_startup_step(buf: &[u8]) -> StartupStep {
    if buf.len() < 8 {
        return StartupStep::Incomplete;
    }
    let len = u32::from_be_bytes([buf[0], buf[1], buf[2], buf[3]]) as usize;
    let code = u32::from_be_bytes([buf[4], buf[5], buf[6], buf[7]]);

    match code {
        SSL_REQUEST_CODE | GSSAPI_REQUEST_CODE if len == 8 => {
            StartupStep::NegotiationRequest { consumed: 8 }
        }
        PROTOCOL_V3 => {
            if len < 8 || buf.len() < len {
                return StartupStep::Incomplete;
            }
            let params = parse_startup_params(&buf[8..len]);
            let user = params.get("user").cloned();
            let database = params.get("database").cloned().or_else(|| user.clone());
            StartupStep::Startup { user, database, consumed: len }
        }
        _ => StartupStep::NotStartup,
    }
}

/// Parse the StartupMessage's key/value parameter block: null-terminated
/// "key\0value\0" pairs, terminated by a final empty key.
fn parse_startup_params(buf: &[u8]) -> HashMap<String, String> {
    let mut map = HashMap::new();
    let mut pos = 0;
    loop {
        if pos >= buf.len() || buf[pos] == 0 {
            break;
        }
        let key_start = pos;
        while pos < buf.len() && buf[pos] != 0 {
            pos += 1;
        }
        let key = String::from_utf8_lossy(&buf[key_start..pos]).into_owned();
        pos += 1; // skip null
        if pos > buf.len() {
            break;
        }
        let val_start = pos;
        while pos < buf.len() && buf[pos] != 0 {
            pos += 1;
        }
        let val = String::from_utf8_lossy(&buf[val_start..pos]).into_owned();
        pos += 1; // skip null
        map.insert(key, val);
    }
    map
}

// ── CommandComplete / ErrorResponse ──────────────────────────────────────────

/// Parse an ErrorResponse's field list: repeated "<1-byte code><string>\0"
/// entries terminated by a final 0x00 byte. Extracts severity ('S') and
/// message ('M').
fn parse_error_fields(payload: &[u8]) -> (String, String) {
    let mut severity = String::new();
    let mut message = String::new();
    let mut pos = 0;
    while pos < payload.len() && payload[pos] != 0 {
        let field_type = payload[pos];
        pos += 1;
        let start = pos;
        while pos < payload.len() && payload[pos] != 0 {
            pos += 1;
        }
        let value = String::from_utf8_lossy(&payload[start..pos]).into_owned();
        if pos < payload.len() {
            pos += 1; // skip null terminator
        }
        match field_type {
            b'S' => severity = value,
            b'M' => message = value,
            _ => {}
        }
    }
    (severity, message)
}

/// Extract the trailing row count from a CommandComplete tag, e.g.
/// "SELECT 5" → 5, "INSERT 0 3" → 3, "UPDATE 2" → 2. Returns `None` for tags
/// with no trailing integer (e.g. "BEGIN", "COMMIT").
fn rows_from_tag(tag: &str) -> Option<u64> {
    tag.split_whitespace().last()?.parse().ok()
}

// ── Session state ─────────────────────────────────────────────────────────────

/// Per-TCP-connection state for an in-progress PostgreSQL session.
/// Held in `SessionManager::pg_sessions`.
#[derive(Debug, Default)]
pub struct PostgresSession {
    client_buf: Vec<u8>,
    client_pos: usize,
    server_buf: Vec<u8>,
    server_pos: usize,
    startup_done: bool,
    /// Set once a negotiation request (SSLRequest/GSSAPIRequest) is seen, so
    /// the server-direction parser knows to skip the single-byte 'S'/'N'
    /// reply before resuming normal message framing.
    expect_negotiation_reply: bool,
    username: Option<String>,
    database: Option<String>,
    pending_query: Option<String>,
}

impl PostgresSession {
    /// Feed client-direction bytes. The simple query protocol carries no
    /// per-query response from the client side, so this never emits flows —
    /// it only updates session state (startup attribution, pending query).
    pub fn push_client(&mut self, data: &[u8]) {
        self.client_buf.extend_from_slice(data);
        loop {
            let buf = &self.client_buf[self.client_pos..];

            if !self.startup_done {
                match read_startup_step(buf) {
                    StartupStep::NegotiationRequest { consumed } => {
                        self.expect_negotiation_reply = true;
                        self.client_pos += consumed;
                        continue;
                    }
                    StartupStep::Startup { user, database, consumed } => {
                        self.username = user;
                        self.database = database;
                        self.startup_done = true;
                        self.client_pos += consumed;
                        continue;
                    }
                    StartupStep::Incomplete => break,
                    StartupStep::NotStartup => {
                        // Give up on attribution; treat the rest of the
                        // stream as normal typed messages.
                        self.startup_done = true;
                        continue;
                    }
                }
            }

            match read_message(buf) {
                Some((FE_QUERY, payload, consumed)) => {
                    if let Some(query) = parse_cstring(payload) {
                        self.pending_query = Some(query);
                    }
                    self.client_pos += consumed;
                }
                Some((_other, _payload, consumed)) => {
                    // Extended-protocol messages (Parse/Bind/Describe/Execute/
                    // Close/Sync), Terminate, or PasswordMessage — skip.
                    // PasswordMessage in particular is intentionally never
                    // inspected: credentials must not reach the Hub.
                    self.client_pos += consumed;
                }
                None => break,
            }
        }
    }

    /// Feed server-direction bytes; returns any completed query flows
    /// (CommandComplete or ErrorResponse matched against the pending query).
    pub fn push_server(&mut self, data: &[u8]) -> Vec<PostgresFlow> {
        self.server_buf.extend_from_slice(data);
        let mut out = Vec::new();

        loop {
            let buf = &self.server_buf[self.server_pos..];

            if self.expect_negotiation_reply {
                if buf.is_empty() {
                    break;
                }
                // Single-byte 'S' (proceed with TLS/GSSAPI) or 'N' (declined).
                self.expect_negotiation_reply = false;
                self.server_pos += 1;
                continue;
            }

            match read_message(buf) {
                Some((BE_COMMAND_COMPLETE, payload, consumed)) => {
                    let tag = parse_cstring(payload).unwrap_or_default();
                    let rows_affected = rows_from_tag(&tag);
                    out.push(PostgresFlow {
                        query: self.pending_query.take(),
                        command_tag: Some(tag),
                        rows_affected,
                        duration_ms: None,
                        error: None,
                        username: self.username.clone(),
                        database: self.database.clone(),
                    });
                    self.server_pos += consumed;
                }
                Some((BE_ERROR_RESPONSE, payload, consumed)) => {
                    let (severity, message) = parse_error_fields(payload);
                    out.push(PostgresFlow {
                        query: self.pending_query.take(),
                        command_tag: None,
                        rows_affected: None,
                        duration_ms: None,
                        error: Some(format!("{}: {}", severity, message)),
                        username: self.username.clone(),
                        database: self.database.clone(),
                    });
                    self.server_pos += consumed;
                }
                Some((_other, _payload, consumed)) => {
                    // Authentication, ParameterStatus, BackendKeyData,
                    // ReadyForQuery, RowDescription, DataRow, NoticeResponse — skip.
                    self.server_pos += consumed;
                }
                None => break,
            }
        }

        out
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Build a StartupMessage: len(4) + protocol version(4) + "key\0value\0"* + \0.
    fn startup_message(params: &[(&str, &str)]) -> Vec<u8> {
        let mut body = Vec::new();
        for (k, v) in params {
            body.extend_from_slice(k.as_bytes());
            body.push(0);
            body.extend_from_slice(v.as_bytes());
            body.push(0);
        }
        body.push(0); // terminator
        let len = (8 + body.len()) as u32;
        let mut msg = Vec::new();
        msg.extend_from_slice(&len.to_be_bytes());
        msg.extend_from_slice(&PROTOCOL_V3.to_be_bytes());
        msg.extend_from_slice(&body);
        msg
    }

    fn ssl_request() -> Vec<u8> {
        let mut msg = Vec::new();
        msg.extend_from_slice(&8u32.to_be_bytes());
        msg.extend_from_slice(&SSL_REQUEST_CODE.to_be_bytes());
        msg
    }

    /// Build a typed message: type(1) + len(4, includes itself) + payload.
    fn typed_message(msg_type: u8, payload: &[u8]) -> Vec<u8> {
        let len = (4 + payload.len()) as u32;
        let mut msg = vec![msg_type];
        msg.extend_from_slice(&len.to_be_bytes());
        msg.extend_from_slice(payload);
        msg
    }

    fn simple_query(sql: &str) -> Vec<u8> {
        let mut payload = sql.as_bytes().to_vec();
        payload.push(0);
        typed_message(FE_QUERY, &payload)
    }

    fn command_complete(tag: &str) -> Vec<u8> {
        let mut payload = tag.as_bytes().to_vec();
        payload.push(0);
        typed_message(BE_COMMAND_COMPLETE, &payload)
    }

    fn error_response(severity: &str, message: &str) -> Vec<u8> {
        let mut payload = Vec::new();
        payload.push(b'S');
        payload.extend_from_slice(severity.as_bytes());
        payload.push(0);
        payload.push(b'M');
        payload.extend_from_slice(message.as_bytes());
        payload.push(0);
        payload.push(0); // terminator
        typed_message(BE_ERROR_RESPONSE, &payload)
    }

    #[test]
    fn is_postgres_port_matches_5432_only() {
        assert!(is_postgres_port(5432));
        assert!(!is_postgres_port(5433));
        assert!(!is_postgres_port(443));
    }

    #[test]
    fn startup_message_extracts_user_and_database() {
        let mut session = PostgresSession::default();
        session.push_client(&startup_message(&[
            ("user", "alice"),
            ("database", "billing"),
            ("application_name", "psql"),
        ]));
        assert_eq!(session.username, Some("alice".to_string()));
        assert_eq!(session.database, Some("billing".to_string()));
        assert!(session.startup_done);
    }

    #[test]
    fn startup_message_defaults_database_to_user() {
        let mut session = PostgresSession::default();
        session.push_client(&startup_message(&[("user", "alice")]));
        assert_eq!(session.username, Some("alice".to_string()));
        assert_eq!(session.database, Some("alice".to_string()));
    }

    #[test]
    fn ssl_request_then_startup_message_still_extracts_attribution() {
        // Most real clients (psql, drivers) send SSLRequest first.
        let mut client_bytes = ssl_request();
        client_bytes.extend_from_slice(&startup_message(&[
            ("user", "bob"),
            ("database", "app"),
        ]));

        let mut session = PostgresSession::default();
        session.push_client(&client_bytes);

        assert!(session.expect_negotiation_reply);
        assert_eq!(session.username, Some("bob".to_string()));
        assert_eq!(session.database, Some("app".to_string()));

        // Server's single-byte 'S' reply must be consumed without producing
        // a spurious flow or breaking subsequent message framing.
        let flows = session.push_server(b"S");
        assert!(flows.is_empty());
        assert!(!session.expect_negotiation_reply);
    }

    #[test]
    fn simple_query_and_command_complete_produce_one_flow() {
        let mut session = PostgresSession::default();
        session.push_client(&startup_message(&[("user", "alice"), ("database", "app")]));
        session.push_client(&simple_query("SELECT * FROM users"));

        let flows = session.push_server(&command_complete("SELECT 5"));

        assert_eq!(flows.len(), 1);
        let flow = &flows[0];
        assert_eq!(flow.query.as_deref(), Some("SELECT * FROM users"));
        assert_eq!(flow.command_tag.as_deref(), Some("SELECT 5"));
        assert_eq!(flow.rows_affected, Some(5));
        assert_eq!(flow.error, None);
        assert_eq!(flow.username.as_deref(), Some("alice"));
        assert_eq!(flow.database.as_deref(), Some("app"));
    }

    #[test]
    fn insert_tag_reports_row_count_not_oid() {
        let mut session = PostgresSession::default();
        session.push_client(&simple_query("INSERT INTO users (name) VALUES ('x')"));
        let flows = session.push_server(&command_complete("INSERT 0 3"));
        assert_eq!(flows[0].rows_affected, Some(3));
    }

    #[test]
    fn tag_with_no_row_count_returns_none() {
        let mut session = PostgresSession::default();
        session.push_client(&simple_query("BEGIN"));
        let flows = session.push_server(&command_complete("BEGIN"));
        assert_eq!(flows[0].rows_affected, None);
    }

    #[test]
    fn error_response_produces_flow_with_error_and_no_command_tag() {
        let mut session = PostgresSession::default();
        session.push_client(&simple_query("SELECT * FROM missing_table"));

        let flows = session.push_server(&error_response("ERROR", "relation \"missing_table\" does not exist"));

        assert_eq!(flows.len(), 1);
        let flow = &flows[0];
        assert_eq!(flow.command_tag, None);
        assert_eq!(flow.rows_affected, None);
        assert_eq!(
            flow.error.as_deref(),
            Some("ERROR: relation \"missing_table\" does not exist")
        );
        assert_eq!(flow.query.as_deref(), Some("SELECT * FROM missing_table"));
    }

    #[test]
    fn incomplete_message_waits_for_more_data() {
        let full = command_complete("SELECT 1");
        let mut session = PostgresSession::default();
        session.push_client(&simple_query("SELECT 1"));

        // Feed only the first 3 bytes — not enough for a full message yet.
        let flows = session.push_server(&full[..3]);
        assert!(flows.is_empty());

        // Feed the rest — the flow should now complete.
        let flows = session.push_server(&full[3..]);
        assert_eq!(flows.len(), 1);
        assert_eq!(flows[0].command_tag.as_deref(), Some("SELECT 1"));
    }

    #[test]
    fn multiple_queries_on_same_connection_each_get_their_own_flow() {
        let mut session = PostgresSession::default();

        session.push_client(&simple_query("SELECT 1"));
        let flows1 = session.push_server(&command_complete("SELECT 1"));
        assert_eq!(flows1.len(), 1);

        session.push_client(&simple_query("SELECT 2"));
        let flows2 = session.push_server(&command_complete("SELECT 1"));
        assert_eq!(flows2.len(), 1);
        assert_eq!(flows2[0].query.as_deref(), Some("SELECT 2"));
    }

    #[test]
    fn password_message_is_never_parsed_into_query_or_leaked() {
        // 'p' PasswordMessage must be skipped, not surfaced as a query.
        let password_msg = typed_message(b'p', b"hunter2\0");
        let mut session = PostgresSession::default();
        session.push_client(&startup_message(&[("user", "alice")]));
        session.push_client(&password_msg);
        session.push_client(&simple_query("SELECT 1"));

        let flows = session.push_server(&command_complete("SELECT 1"));
        assert_eq!(flows[0].query.as_deref(), Some("SELECT 1"));
        // Nothing about the password should appear anywhere in the flow.
        let serialized = format!("{:?}", flows[0]);
        assert!(!serialized.contains("hunter2"));
    }
}
