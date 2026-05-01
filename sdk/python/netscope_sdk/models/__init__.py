"""NetScope SDK — data models (plain dataclasses, no third-party deps)."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from typing import Any


# ── Helpers ───────────────────────────────────────────────────────────────────

def _dt(val: str | None) -> datetime | None:
    if not val:
        return None
    # Handle both "Z" suffix and "+00:00" offset
    val = val.replace("Z", "+00:00")
    try:
        return datetime.fromisoformat(val)
    except ValueError:
        return None


# ── Sub-models ────────────────────────────────────────────────────────────────

@dataclass
class HttpFlow:
    method: str = ""
    path: str = ""
    status: int = 0
    latency_ms: int = 0

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "HttpFlow":
        return cls(
            method=d.get("method", ""),
            path=d.get("path", ""),
            status=d.get("status", 0),
            latency_ms=d.get("latency_ms", 0),
        )


@dataclass
class DnsFlow:
    query_name: str = ""
    query_type: str = ""
    is_response: bool = False
    answers: list[str] = field(default_factory=list)
    rcode: int = 0

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "DnsFlow":
        return cls(
            query_name=d.get("query_name", ""),
            query_type=d.get("query_type", ""),
            is_response=d.get("is_response", False),
            answers=d.get("answers") or [],
            rcode=d.get("rcode", 0),
        )


@dataclass
class TlsFlow:
    sni: str = ""
    version: str = ""
    negotiated_version: str = ""
    chosen_cipher: str = ""
    has_weak_cipher: bool = False
    cert_cn: str = ""
    cert_sans: list[str] = field(default_factory=list)
    cert_expiry: str = ""
    cert_expired: bool = False
    cert_issuer: str = ""
    alert_level: str = ""
    alert_description: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "TlsFlow":
        return cls(
            sni=d.get("sni", ""),
            version=d.get("version", ""),
            negotiated_version=d.get("negotiated_version", ""),
            chosen_cipher=d.get("chosen_cipher", ""),
            has_weak_cipher=d.get("has_weak_cipher", False),
            cert_cn=d.get("cert_cn", ""),
            cert_sans=d.get("cert_sans") or [],
            cert_expiry=d.get("cert_expiry", ""),
            cert_expired=d.get("cert_expired", False),
            cert_issuer=d.get("cert_issuer", ""),
            alert_level=d.get("alert_level", ""),
            alert_description=d.get("alert_description", ""),
        )


# ── Core models ───────────────────────────────────────────────────────────────

@dataclass
class Flow:
    id: str = ""
    agent_id: str = ""
    hostname: str = ""
    timestamp: datetime | None = None
    protocol: str = ""
    src_ip: str = ""
    src_port: int = 0
    dst_ip: str = ""
    dst_port: int = 0
    bytes_in: int = 0
    bytes_out: int = 0
    duration_ms: int = 0
    info: str = ""
    process_name: str = ""
    pid: int = 0
    country_code: str = ""
    country_name: str = ""
    asn: str = ""
    org: str = ""
    threat_score: int = 0
    threat_level: str = ""
    http: HttpFlow | None = None
    dns: DnsFlow | None = None
    tls: TlsFlow | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Flow":
        return cls(
            id=d.get("id", ""),
            agent_id=d.get("agent_id", ""),
            hostname=d.get("hostname", ""),
            timestamp=_dt(d.get("timestamp")),
            protocol=d.get("protocol", ""),
            src_ip=d.get("src_ip", ""),
            src_port=d.get("src_port", 0),
            dst_ip=d.get("dst_ip", ""),
            dst_port=d.get("dst_port", 0),
            bytes_in=d.get("bytes_in", 0),
            bytes_out=d.get("bytes_out", 0),
            duration_ms=d.get("duration_ms", 0),
            info=d.get("info", ""),
            process_name=d.get("process_name", ""),
            pid=d.get("pid", 0),
            country_code=d.get("country_code", ""),
            country_name=d.get("country_name", ""),
            asn=d.get("asn", ""),
            org=d.get("org", ""),
            threat_score=d.get("threat_score", 0),
            threat_level=d.get("threat_level", ""),
            http=HttpFlow.from_dict(d["http"]) if d.get("http") else None,
            dns=DnsFlow.from_dict(d["dns"]) if d.get("dns") else None,
            tls=TlsFlow.from_dict(d["tls"]) if d.get("tls") else None,
        )

    @property
    def total_bytes(self) -> int:
        return self.bytes_in + self.bytes_out

    @property
    def is_threat(self) -> bool:
        return self.threat_level in ("high", "medium")


@dataclass
class AlertRule:
    id: str = ""
    name: str = ""
    description: str = ""
    condition: str = ""
    severity: str = ""
    enabled: bool = True
    integration: str = ""
    webhook_url: str = ""
    email_to: str = ""
    webhook_secret: str = ""
    created_at: datetime | None = None
    updated_at: datetime | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "AlertRule":
        return cls(
            id=d.get("id", ""),
            name=d.get("name", ""),
            description=d.get("description", ""),
            condition=d.get("condition", ""),
            severity=d.get("severity", ""),
            enabled=d.get("enabled", True),
            integration=d.get("integration", ""),
            webhook_url=d.get("webhook_url", ""),
            email_to=d.get("email_to", ""),
            webhook_secret=d.get("webhook_secret", ""),
            created_at=_dt(d.get("created_at")),
            updated_at=_dt(d.get("updated_at")),
        )


@dataclass
class AlertEvent:
    id: str = ""
    rule_id: str = ""
    rule_name: str = ""
    severity: str = ""
    fired_at: datetime | None = None
    detail: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "AlertEvent":
        return cls(
            id=d.get("id", ""),
            rule_id=d.get("rule_id", ""),
            rule_name=d.get("rule_name", ""),
            severity=d.get("severity", ""),
            fired_at=_dt(d.get("fired_at")),
            detail=d.get("detail", ""),
        )


@dataclass
class Anomaly:
    id: str = ""
    src_ip: str = ""
    dst_ip: str = ""
    protocol: str = ""
    score: float = 0.0
    severity: str = ""
    description: str = ""
    detected_at: datetime | None = None
    agent_id: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Anomaly":
        return cls(
            id=d.get("id", ""),
            src_ip=d.get("src_ip", ""),
            dst_ip=d.get("dst_ip", ""),
            protocol=d.get("protocol", ""),
            score=float(d.get("score", 0.0)),
            severity=d.get("severity", ""),
            description=d.get("description", ""),
            detected_at=_dt(d.get("detected_at")),
            agent_id=d.get("agent_id", ""),
        )


@dataclass
class Agent:
    id: str = ""
    label: str = ""
    os: str = ""
    capture_mode: str = ""
    version: str = ""
    last_seen: datetime | None = None
    flow_count_1h: int = 0
    ebpf_enabled: bool = False
    labels: dict[str, str] = field(default_factory=dict)
    config_version: int = 0

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Agent":
        return cls(
            id=d.get("id", ""),
            label=d.get("label", ""),
            os=d.get("os", ""),
            capture_mode=d.get("capture_mode", ""),
            version=d.get("version", ""),
            last_seen=_dt(d.get("last_seen")),
            flow_count_1h=d.get("flow_count_1h", 0),
            ebpf_enabled=d.get("ebpf_enabled", False),
            labels=d.get("labels") or {},
            config_version=d.get("config_version", 0),
        )

    @property
    def is_online(self) -> bool:
        if not self.last_seen:
            return False
        from datetime import timezone
        delta = datetime.now(timezone.utc) - self.last_seen
        return delta.total_seconds() < 120  # online if seen in last 2 min


@dataclass
class Widget:
    id: str = ""
    type: str = ""
    title: str = ""
    size: str = "md"
    config: dict[str, Any] = field(default_factory=dict)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Widget":
        return cls(
            id=d.get("id", ""),
            type=d.get("type", ""),
            title=d.get("title", ""),
            size=d.get("size", "md"),
            config=d.get("config") or {},
        )

    def to_dict(self) -> dict[str, Any]:
        return {"id": self.id, "type": self.type, "title": self.title,
                "size": self.size, "config": self.config}


@dataclass
class Dashboard:
    id: str = ""
    name: str = ""
    description: str = ""
    widgets: list[Widget] = field(default_factory=list)
    created_at: datetime | None = None
    updated_at: datetime | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Dashboard":
        widgets_raw = d.get("widgets") or []
        if isinstance(widgets_raw, str):
            import json
            try:
                widgets_raw = json.loads(widgets_raw)
            except Exception:
                widgets_raw = []
        return cls(
            id=d.get("id", ""),
            name=d.get("name", ""),
            description=d.get("description", ""),
            widgets=[Widget.from_dict(w) for w in widgets_raw],
            created_at=_dt(d.get("created_at")),
            updated_at=_dt(d.get("updated_at")),
        )


@dataclass
class Incident:
    id: str = ""
    title: str = ""
    description: str = ""
    severity: str = ""
    status: str = ""
    assignee: str = ""
    notes: str = ""
    created_at: datetime | None = None
    updated_at: datetime | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Incident":
        return cls(
            id=d.get("id", ""),
            title=d.get("title", ""),
            description=d.get("description", ""),
            severity=d.get("severity", ""),
            status=d.get("status", ""),
            assignee=d.get("assignee", ""),
            notes=d.get("notes", ""),
            created_at=_dt(d.get("created_at")),
            updated_at=_dt(d.get("updated_at")),
        )


@dataclass
class SigmaRule:
    id: str = ""
    title: str = ""
    description: str = ""
    level: str = ""
    status: str = ""
    enabled: bool = True
    rule_yaml: str = ""
    builtin: bool = False
    match_count: int = 0
    created_at: datetime | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "SigmaRule":
        return cls(
            id=d.get("id", ""),
            title=d.get("title", ""),
            description=d.get("description", ""),
            level=d.get("level", ""),
            status=d.get("status", ""),
            enabled=d.get("enabled", True),
            rule_yaml=d.get("rule_yaml", ""),
            builtin=d.get("builtin", False),
            match_count=d.get("match_count", 0),
            created_at=_dt(d.get("created_at")),
        )


@dataclass
class Stats:
    total_flows: int = 0
    flows_per_minute: float = 0.0
    active_agents: int = 0
    bytes_in_24h: int = 0
    bytes_out_24h: int = 0
    anomaly_count_24h: int = 0
    alert_count_24h: int = 0

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "Stats":
        return cls(
            total_flows=d.get("total_flows", 0),
            flows_per_minute=float(d.get("flows_per_minute", 0.0)),
            active_agents=d.get("active_agents", 0),
            bytes_in_24h=d.get("bytes_in_24h", 0),
            bytes_out_24h=d.get("bytes_out_24h", 0),
            anomaly_count_24h=d.get("anomaly_count_24h", 0),
            alert_count_24h=d.get("alert_count_24h", 0),
        )


@dataclass
class TimeseriesPoint:
    ts: datetime | None = None
    flows: int = 0
    bytes_in: int = 0
    bytes_out: int = 0

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> "TimeseriesPoint":
        return cls(
            ts=_dt(d.get("ts")),
            flows=d.get("flows", 0),
            bytes_in=d.get("bytes_in", 0),
            bytes_out=d.get("bytes_out", 0),
        )


__all__ = [
    "Flow", "HttpFlow", "DnsFlow", "TlsFlow",
    "AlertRule", "AlertEvent",
    "Anomaly",
    "Agent",
    "Widget", "Dashboard",
    "Incident",
    "SigmaRule",
    "Stats", "TimeseriesPoint",
]
