"""Unit tests for SDK data models (no Hub required)."""

from __future__ import annotations

from datetime import timezone
from netscope_sdk.models import (
    Flow, HttpFlow, DnsFlow, TlsFlow,
    AlertRule, AlertEvent, Anomaly, Agent,
    Dashboard, Widget, Incident, SigmaRule,
    Stats, TimeseriesPoint,
)


class TestFlow:
    def test_from_dict_minimal(self):
        f = Flow.from_dict({"id": "abc", "src_ip": "10.0.0.1", "dst_ip": "8.8.8.8"})
        assert f.id == "abc"
        assert f.src_ip == "10.0.0.1"
        assert f.http is None
        assert f.dns is None

    def test_total_bytes(self):
        f = Flow.from_dict({"bytes_in": 100, "bytes_out": 200})
        assert f.total_bytes == 300

    def test_is_threat_high(self):
        f = Flow.from_dict({"threat_level": "high"})
        assert f.is_threat is True

    def test_is_threat_low(self):
        f = Flow.from_dict({"threat_level": "low"})
        assert f.is_threat is False

    def test_is_threat_none(self):
        f = Flow.from_dict({})
        assert f.is_threat is False

    def test_nested_tls(self):
        f = Flow.from_dict({"tls": {"sni": "example.com", "cert_expired": True}})
        assert f.tls is not None
        assert f.tls.sni == "example.com"
        assert f.tls.cert_expired is True

    def test_nested_dns(self):
        f = Flow.from_dict({"dns": {"query_name": "google.com", "answers": ["142.250.0.1"]}})
        assert f.dns is not None
        assert f.dns.query_name == "google.com"
        assert f.dns.answers == ["142.250.0.1"]

    def test_nested_http(self):
        f = Flow.from_dict({"http": {"method": "GET", "path": "/api/health", "status": 200}})
        assert f.http is not None
        assert f.http.method == "GET"
        assert f.http.status == 200

    def test_timestamp_parsing(self):
        f = Flow.from_dict({"timestamp": "2025-01-15T10:30:00Z"})
        assert f.timestamp is not None
        assert f.timestamp.tzinfo is not None


class TestAgent:
    def test_is_online_recent(self):
        from datetime import datetime, timedelta
        now = datetime.now(timezone.utc)
        recent = (now - timedelta(seconds=30)).isoformat()
        a = Agent.from_dict({"last_seen": recent})
        assert a.is_online is True

    def test_is_online_stale(self):
        from datetime import datetime, timedelta
        old = (datetime.now(timezone.utc) - timedelta(hours=2)).isoformat()
        a = Agent.from_dict({"last_seen": old})
        assert a.is_online is False

    def test_is_online_never(self):
        a = Agent.from_dict({})
        assert a.is_online is False


class TestDashboard:
    def test_widgets_parsed(self):
        d = Dashboard.from_dict({
            "id": "d1",
            "name": "Test",
            "widgets": [
                {"id": "w1", "type": "stat", "title": "Flows", "size": "sm",
                 "config": {"metric": "total_flows"}},
            ],
        })
        assert len(d.widgets) == 1
        assert d.widgets[0].type == "stat"
        assert d.widgets[0].config["metric"] == "total_flows"

    def test_widgets_empty_json_string(self):
        import json
        d = Dashboard.from_dict({"widgets": json.dumps([])})
        assert d.widgets == []

    def test_widget_to_dict_roundtrip(self):
        w = Widget(id="w1", type="timeseries", title="Volume", size="lg",
                   config={"window": "24h"})
        assert Widget.from_dict(w.to_dict()) == w


class TestStats:
    def test_from_dict(self):
        s = Stats.from_dict({
            "total_flows": 100_000,
            "flows_per_minute": 12.5,
            "active_agents": 3,
        })
        assert s.total_flows == 100_000
        assert s.flows_per_minute == 12.5
        assert s.active_agents == 3


class TestIncident:
    def test_from_dict(self):
        i = Incident.from_dict({
            "id": "INC-001",
            "title": "Test incident",
            "severity": "P2",
            "status": "open",
        })
        assert i.severity == "P2"
        assert i.status == "open"


class TestExceptions:
    def test_raise_400(self):
        from netscope_sdk.exceptions import ValidationError, _raise_for_status
        try:
            _raise_for_status(400, "bad request")
            assert False, "should have raised"
        except ValidationError as e:
            assert e.status_code == 400

    def test_raise_401(self):
        from netscope_sdk.exceptions import AuthError, _raise_for_status
        try:
            _raise_for_status(401, "unauthorized")
        except AuthError as e:
            assert e.status_code == 401

    def test_raise_403(self):
        from netscope_sdk.exceptions import ForbiddenError, _raise_for_status
        try:
            _raise_for_status(403, "forbidden")
        except ForbiddenError:
            pass

    def test_raise_404(self):
        from netscope_sdk.exceptions import NotFoundError, _raise_for_status
        try:
            _raise_for_status(404, "not found")
        except NotFoundError:
            pass

    def test_raise_500(self):
        from netscope_sdk.exceptions import ServerError, _raise_for_status
        try:
            _raise_for_status(500, "internal error")
        except ServerError as e:
            assert e.status_code == 500

    def test_200_no_raise(self):
        from netscope_sdk.exceptions import _raise_for_status
        # Should not raise for 2xx
        _raise_for_status(200, "ok")  # no exception
