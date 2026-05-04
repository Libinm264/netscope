/// PII masking engine — redacts sensitive data before flows leave the agent.
///
/// Applied to every HTTP/1.1 and HTTP/2 flow immediately after parsing, so
/// that no credentials, tokens, or personal data reach the ClickHouse Hub.
///
/// # What gets redacted
///
/// **Headers** — values of headers whose names appear in `SENSITIVE_HEADERS`
/// are replaced with `"[REDACTED]"`. The match is case-insensitive so
/// `Authorization`, `AUTHORIZATION`, and `authorization` are all caught.
///
/// **Body fields** — when the body preview is valid JSON, any field whose
/// name appears in `SENSITIVE_FIELDS` has its value replaced with
/// `"[REDACTED]"`. The walk is recursive so nested objects are also masked.
/// Non-JSON bodies (form-encoded, binary, plain text) are returned as-is.
///
/// # Adding new patterns
///
/// Add the lowercase name to `SENSITIVE_HEADERS` or `SENSITIVE_FIELDS`.
/// The engine picks it up automatically — no other changes needed.

pub const REDACTED: &str = "[REDACTED]";

/// HTTP header names whose values must always be redacted (lowercase).
const SENSITIVE_HEADERS: &[&str] = &[
    "authorization",
    "proxy-authorization",
    "cookie",
    "set-cookie",
    "x-api-key",
    "x-auth-token",
    "x-access-token",
    "x-session-token",
    "x-csrf-token",
    "x-forwarded-authorization",
    "api-key",
    "x-amz-security-token",
    "x-goog-api-key",
];

/// JSON body field names whose values must be redacted (lowercase).
const SENSITIVE_FIELDS: &[&str] = &[
    // auth / credentials
    "password",
    "passwd",
    "pass",
    "secret",
    "token",
    "api_key",
    "apikey",
    "api-key",
    "access_token",
    "refresh_token",
    "id_token",
    "client_secret",
    "client_id",
    "auth_token",
    "session_token",
    "session_id",
    "authorization",
    "signature",
    "private_key",
    "public_key",
    "aws_secret_access_key",
    "aws_session_token",
    // payment / PAN
    "card_number",
    "cardnumber",
    "card_num",
    "pan",
    "cvv",
    "cvc",
    "cvv2",
    "cvc2",
    "expiry",
    "expiration",
    "card_expiry",
    "credit_card",
    // personal / identity
    "ssn",
    "social_security",
    "social_security_number",
    "dob",
    "date_of_birth",
    "national_id",
    "passport",
    "driving_license",
    // banking
    "account_number",
    "routing_number",
    "iban",
    "bic",
    "swift",
];

// ── Public API ────────────────────────────────────────────────────────────────

/// Redact the values of sensitive headers in-place.
///
/// ```rust
/// use parser::masking::{mask_headers, REDACTED};
///
/// let mut headers = vec![
///     ("Host".to_string(),          "api.example.com".to_string()),
///     ("Authorization".to_string(), "Bearer super-secret".to_string()),
///     ("Content-Type".to_string(),  "application/json".to_string()),
/// ];
/// mask_headers(&mut headers);
/// assert_eq!(headers[1].1, REDACTED);
/// assert_eq!(headers[0].1, "api.example.com"); // untouched
/// assert_eq!(headers[2].1, "application/json"); // untouched
/// ```
pub fn mask_headers(headers: &mut Vec<(String, String)>) {
    for (name, value) in headers.iter_mut() {
        if is_sensitive_header(name) {
            *value = REDACTED.to_string();
        }
    }
}

/// Redact sensitive field values in a body preview string.
///
/// - If the string is valid JSON, walks the object recursively and redacts
///   any field whose name appears in `SENSITIVE_FIELDS`.
/// - Non-JSON bodies are returned unchanged.
/// - `None` is returned as `None`.
pub fn mask_body(body: Option<String>) -> Option<String> {
    let raw = body?;
    if raw.is_empty() {
        return Some(raw);
    }
    match serde_json::from_str::<serde_json::Value>(&raw) {
        Ok(mut v) => {
            mask_json_value(&mut v);
            Some(serde_json::to_string(&v).unwrap_or(raw))
        }
        // Not JSON — leave as-is (form-encoded, plain text, binary preview, etc.)
        Err(_) => Some(raw),
    }
}

// ── Internals ─────────────────────────────────────────────────────────────────

fn is_sensitive_header(name: &str) -> bool {
    let lower = name.to_ascii_lowercase();
    SENSITIVE_HEADERS.contains(&lower.as_str())
}

fn is_sensitive_field(name: &str) -> bool {
    let lower = name.to_ascii_lowercase();
    SENSITIVE_FIELDS.contains(&lower.as_str())
}

/// Recursively walk a JSON value and redact any object field whose name
/// is in `SENSITIVE_FIELDS`.
fn mask_json_value(v: &mut serde_json::Value) {
    match v {
        serde_json::Value::Object(map) => {
            for (key, val) in map.iter_mut() {
                if is_sensitive_field(key) {
                    *val = serde_json::Value::String(REDACTED.to_string());
                } else {
                    mask_json_value(val); // recurse into nested objects
                }
            }
        }
        serde_json::Value::Array(arr) => {
            for item in arr.iter_mut() {
                mask_json_value(item);
            }
        }
        _ => {} // scalars are left alone
    }
}

// ── Tests ─────────────────────────────────────────────────────────────────────

#[cfg(test)]
mod tests {
    use super::*;

    // ── Header masking ────────────────────────────────────────────────────────

    #[test]
    fn masks_authorization_header() {
        let mut h = vec![
            ("Authorization".into(), "Bearer eyJhbGc.abc.def".into()),
        ];
        mask_headers(&mut h);
        assert_eq!(h[0].1, REDACTED);
    }

    #[test]
    fn header_masking_is_case_insensitive() {
        let mut h = vec![
            ("AUTHORIZATION".into(), "secret".into()),
            ("x-Api-Key".into(),     "key123".into()),
            ("X-AUTH-TOKEN".into(),  "tok".into()),
        ];
        mask_headers(&mut h);
        assert!(h.iter().all(|(_, v)| v == REDACTED));
    }

    #[test]
    fn non_sensitive_headers_are_untouched() {
        let mut h = vec![
            ("Host".into(),         "api.example.com".into()),
            ("Content-Type".into(), "application/json".into()),
            ("Accept".into(),       "*/*".into()),
        ];
        mask_headers(&mut h);
        assert_eq!(h[0].1, "api.example.com");
        assert_eq!(h[1].1, "application/json");
        assert_eq!(h[2].1, "*/*");
    }

    #[test]
    fn masks_cookie_and_set_cookie() {
        let mut h = vec![
            ("Cookie".into(),     "session=abc123; user=alice".into()),
            ("Set-Cookie".into(), "session=xyz; HttpOnly".into()),
        ];
        mask_headers(&mut h);
        assert_eq!(h[0].1, REDACTED);
        assert_eq!(h[1].1, REDACTED);
    }

    #[test]
    fn masks_proxy_authorization() {
        let mut h = vec![
            ("Proxy-Authorization".into(), "Basic dXNlcjpwYXNz".into()),
        ];
        mask_headers(&mut h);
        assert_eq!(h[0].1, REDACTED);
    }

    #[test]
    fn empty_headers_vec_is_safe() {
        let mut h: Vec<(String, String)> = vec![];
        mask_headers(&mut h);
        assert!(h.is_empty());
    }

    // ── Body / JSON masking ───────────────────────────────────────────────────

    #[test]
    fn masks_password_field_in_json() {
        let body = r#"{"username":"alice","password":"hunter2"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["username"], "alice");
        assert_eq!(v["password"], REDACTED);
    }

    #[test]
    fn masks_token_and_api_key_fields() {
        let body = r#"{"token":"abc123","api_key":"key-xyz","data":"ok"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["token"],   REDACTED);
        assert_eq!(v["api_key"], REDACTED);
        assert_eq!(v["data"],    "ok");
    }

    #[test]
    fn masks_payment_fields() {
        let body = r#"{"amount":99,"card_number":"4111111111111111","cvv":"123"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["card_number"], REDACTED);
        assert_eq!(v["cvv"],        REDACTED);
        assert_eq!(v["amount"],     99);
    }

    #[test]
    fn masks_nested_json_fields() {
        let body = r#"{"user":{"email":"a@b.com","password":"s3cret"},"ok":true}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["user"]["email"],    "a@b.com");
        assert_eq!(v["user"]["password"], REDACTED);
        assert_eq!(v["ok"],              true);
    }

    #[test]
    fn masks_fields_inside_json_array() {
        let body = r#"[{"id":1,"secret":"x"},{"id":2,"secret":"y"}]"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v[0]["id"],     1);
        assert_eq!(v[0]["secret"], REDACTED);
        assert_eq!(v[1]["secret"], REDACTED);
    }

    #[test]
    fn field_masking_is_case_insensitive() {
        // "Password" with capital P
        let body = r#"{"Password":"s3cret","Token":"abc"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["Password"], REDACTED);
        assert_eq!(v["Token"],    REDACTED);
    }

    #[test]
    fn non_json_body_returned_unchanged() {
        let body = "username=alice&password=hunter2"; // URL-encoded, not JSON
        let result = mask_body(Some(body.into())).unwrap();
        // Non-JSON: returned as-is (no masking attempt)
        assert_eq!(result, body);
    }

    #[test]
    fn none_body_returns_none() {
        assert!(mask_body(None).is_none());
    }

    #[test]
    fn empty_body_returned_unchanged() {
        let result = mask_body(Some(String::new())).unwrap();
        assert!(result.is_empty());
    }

    #[test]
    fn plain_json_scalar_body_unchanged() {
        // A bare string JSON value — no fields to mask
        let body = r#""just a string""#;
        let result = mask_body(Some(body.into())).unwrap();
        assert_eq!(result, body);
    }

    #[test]
    fn masks_ssn_and_passport() {
        let body = r#"{"name":"Bob","ssn":"123-45-6789","passport":"AB1234567"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["ssn"],      REDACTED);
        assert_eq!(v["passport"], REDACTED);
        assert_eq!(v["name"],     "Bob");
    }

    #[test]
    fn unrelated_json_fields_are_untouched() {
        let body = r#"{"user_id":42,"email":"user@example.com","role":"admin"}"#;
        let result = mask_body(Some(body.into())).unwrap();
        let v: serde_json::Value = serde_json::from_str(&result).unwrap();
        assert_eq!(v["user_id"], 42);
        assert_eq!(v["email"],   "user@example.com");
        assert_eq!(v["role"],    "admin");
    }
}
