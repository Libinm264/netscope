pub mod dns;
pub mod http;
pub mod http2;
pub mod session;
pub mod tls;

use proto::FlowPayload;

/// Result of attempting to parse a buffer as a known application protocol.
#[derive(Debug)]
pub enum ParseResult {
    /// Successfully parsed; includes the decoded flow payload and bytes consumed.
    Complete(FlowPayload, usize),
    /// Need more data to complete parsing (e.g. partial HTTP response body).
    Incomplete,
    /// The buffer does not match this protocol.
    NotMatched,
}
