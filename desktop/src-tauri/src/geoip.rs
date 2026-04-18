/// GeoIP enrichment using MaxMind GeoLite2 databases.
///
/// Databases are looked up from `~/.netscope/` by default.
/// Download free copies from https://dev.maxmind.com/geoip/geolite2-free-geolocation-data
use maxminddb::{geoip2, MaxMindDBError, Reader};
use std::net::IpAddr;
use std::path::{Path, PathBuf};

pub struct GeoIpReader {
    city: Option<Reader<Vec<u8>>>,
    asn: Option<Reader<Vec<u8>>>,
}

#[derive(Debug, Clone)]
pub struct GeoInfo {
    pub country_code: String,
    pub country_name: String,
    pub city: String,
    pub asn: u32,
    pub as_org: String,
}

fn is_private(ip: &IpAddr) -> bool {
    match ip {
        IpAddr::V4(v4) => {
            v4.is_private()
                || v4.is_loopback()
                || v4.is_link_local()
                || v4.is_broadcast()
                || v4.is_unspecified()
        }
        IpAddr::V6(v6) => v6.is_loopback() || v6.is_unspecified(),
    }
}

impl GeoIpReader {
    /// Open databases at the given paths. Either path may be absent — we degrade
    /// gracefully to whichever DB is available.
    pub fn open(city_path: &Path, asn_path: &Path) -> Result<Self, String> {
        let city = if city_path.exists() {
            Some(
                Reader::open_readfile(city_path)
                    .map_err(|e| format!("Failed to open GeoLite2-City: {e}"))?,
            )
        } else {
            None
        };

        let asn = if asn_path.exists() {
            Some(
                Reader::open_readfile(asn_path)
                    .map_err(|e| format!("Failed to open GeoLite2-ASN: {e}"))?,
            )
        } else {
            None
        };

        if city.is_none() && asn.is_none() {
            return Err("No GeoIP databases found".to_string());
        }

        Ok(GeoIpReader { city, asn })
    }

    /// Attempt to load databases from the default location (`~/.netscope/`).
    pub fn try_default() -> Option<Self> {
        let base = default_db_dir()?;
        Self::open(
            &base.join("GeoLite2-City.mmdb"),
            &base.join("GeoLite2-ASN.mmdb"),
        )
        .ok()
    }

    /// Resolve geographic info for a single IP string. Returns `None` for
    /// private/loopback addresses or any parse/lookup failure.
    pub fn lookup(&self, ip_str: &str) -> Option<GeoInfo> {
        let ip: IpAddr = ip_str.parse().ok()?;
        if is_private(&ip) {
            return None;
        }

        // ── City / country ────────────────────────────────────────────────────
        let (country_code, country_name, city_name) = match &self.city {
            Some(city_db) => match city_db.lookup::<geoip2::City>(ip) {
                Ok(rec) => {
                    let cc = rec
                        .country
                        .as_ref()
                        .and_then(|c| c.iso_code)
                        .unwrap_or("??")
                        .to_string();
                    let name = rec
                        .country
                        .as_ref()
                        .and_then(|c| c.names.as_ref())
                        .and_then(|n| n.get("en").copied())
                        .unwrap_or("Unknown")
                        .to_string();
                    let city = rec
                        .city
                        .as_ref()
                        .and_then(|c| c.names.as_ref())
                        .and_then(|n| n.get("en").copied())
                        .unwrap_or("")
                        .to_string();
                    (cc, name, city)
                }
                Err(MaxMindDBError::AddressNotFoundError(_)) => {
                    ("??".to_string(), "Unknown".to_string(), String::new())
                }
                Err(_) => return None,
            },
            None => ("??".to_string(), "Unknown".to_string(), String::new()),
        };

        // ── ASN ───────────────────────────────────────────────────────────────
        let (asn, as_org) = match &self.asn {
            Some(asn_db) => match asn_db.lookup::<geoip2::Asn>(ip) {
                Ok(rec) => (
                    rec.autonomous_system_number.unwrap_or(0),
                    rec.autonomous_system_organization
                        .unwrap_or("")
                        .to_string(),
                ),
                Err(_) => (0, String::new()),
            },
            None => (0, String::new()),
        };

        Some(GeoInfo {
            country_code,
            country_name,
            city: city_name,
            asn,
            as_org,
        })
    }
}

/// `~/.netscope/`
pub fn default_db_dir() -> Option<PathBuf> {
    dirs::home_dir().map(|h| h.join(".netscope"))
}
