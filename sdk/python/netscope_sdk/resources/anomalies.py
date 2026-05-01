"""NetScope SDK — Anomalies resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import Anomaly

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class AnomaliesResource:
    """
    Query anomaly detection events.

    Example::

        highs = client.anomalies.list(severity="high", hours=24)
        for a in highs:
            print(f"[{a.severity.upper()}] {a.src_ip} → {a.dst_ip}: {a.description}")
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    def list(
        self,
        *,
        severity: str | None = None,
        src_ip: str | None = None,
        protocol: str | None = None,
        agent_id: str | None = None,
        hours: int | None = None,
        limit: int = 100,
        offset: int = 0,
    ) -> list[Anomaly]:
        """
        Return anomaly events matching the given filters.

        :param severity:  ``"high"``, ``"medium"``, or ``"low"``
        :param src_ip:    Source IP to filter on
        :param protocol:  Protocol string (e.g. ``"DNS"``)
        :param agent_id:  UUID of the reporting agent
        :param hours:     Look-back window in hours
        :param limit:     Max rows (default 100)
        :param offset:    Pagination offset
        """
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if severity:
            params["severity"] = severity
        if src_ip:
            params["src_ip"] = src_ip
        if protocol:
            params["protocol"] = protocol
        if agent_id:
            params["agent_id"] = agent_id
        if hours is not None:
            params["hours"] = hours

        data = self._c._get("/api/v1/anomalies", params=params)
        return [Anomaly.from_dict(a) for a in (data or [])]

    def stats(self) -> dict[str, Any]:
        """
        Return aggregated anomaly counts by severity for the last 24 hours.

        :returns: Dict with keys ``"high"``, ``"medium"``, ``"low"``, ``"total"``
        """
        return self._c._get("/api/v1/anomalies/stats") or {}

    def get_baseline(self, src_ip: str, dst_ip: str, protocol: str) -> dict[str, Any]:
        """
        Return the statistical baseline for a specific (src, dst, protocol) tuple.

        :returns: Dict with ``"mean"``, ``"stddev"``, ``"samples"`` etc.
        """
        params = {"src_ip": src_ip, "dst_ip": dst_ip, "protocol": protocol}
        return self._c._get("/api/v1/anomalies/baseline", params=params) or {}
