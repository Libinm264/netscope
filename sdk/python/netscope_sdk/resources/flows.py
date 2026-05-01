"""NetScope SDK — Flows resource."""

from __future__ import annotations

from collections.abc import Generator
from typing import TYPE_CHECKING, Any

from netscope_sdk.models import Flow, TimeseriesPoint

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class FlowsResource:
    """
    Access the /api/v1/flows endpoints.

    Example::

        flows = client.flows.list(protocol="TLS", limit=100)
        for f in flows:
            print(f.src_ip, "→", f.dst_ip, f.tls.sni if f.tls else "")
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    # ── Query ─────────────────────────────────────────────────────────────────

    def list(
        self,
        *,
        protocol: str | None = None,
        src_ip: str | None = None,
        dst_ip: str | None = None,
        port: int | None = None,
        country: str | None = None,
        threat_level: str | None = None,
        agent_id: str | None = None,
        hours: int | None = None,
        limit: int = 200,
        offset: int = 0,
    ) -> list[Flow]:
        """
        Query flows with optional filters.

        :param protocol: e.g. ``"TLS"``, ``"DNS"``, ``"HTTP"``
        :param src_ip:   Source IP (exact or CIDR)
        :param dst_ip:   Destination IP (exact or CIDR)
        :param port:     Match src or dst port
        :param country:  ISO 3166-1 alpha-2 country code, e.g. ``"CN"``
        :param threat_level: ``"high"``, ``"medium"``, ``"low"``
        :param agent_id: Filter to a specific agent UUID
        :param hours:    Look back N hours (default: all available)
        :param limit:    Max rows to return (default 200, max 10 000)
        :param offset:   Pagination offset
        :returns: List of :class:`~netscope_sdk.models.Flow` objects
        """
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if protocol:
            params["protocol"] = protocol
        if src_ip:
            params["src_ip"] = src_ip
        if dst_ip:
            params["dst_ip"] = dst_ip
        if port is not None:
            params["port"] = port
        if country:
            params["country"] = country
        if threat_level:
            params["threat_level"] = threat_level
        if agent_id:
            params["agent_id"] = agent_id
        if hours is not None:
            params["hours"] = hours

        data = self._c._get("/api/v1/flows", params=params)
        return [Flow.from_dict(f) for f in (data or [])]

    def iter_all(
        self,
        *,
        page_size: int = 500,
        **kwargs: Any,
    ) -> Generator[Flow, None, None]:
        """
        Lazily iterate over **all** matching flows, auto-paginating.

        Example::

            for flow in client.flows.iter_all(protocol="DNS", hours=24):
                process(flow)
        """
        offset = 0
        while True:
            page = self.list(limit=page_size, offset=offset, **kwargs)
            yield from page
            if len(page) < page_size:
                break
            offset += page_size

    def stream(self) -> Generator[Flow, None, None]:
        """
        Open a Server-Sent Events stream and yield flows in real time.
        Blocks until the caller breaks out of the loop or the connection drops.

        Example::

            for flow in client.flows.stream():
                if flow.is_threat:
                    alert(flow)
        """
        for data in self._c._sse("/api/v1/flows/stream"):
            try:
                import json
                yield Flow.from_dict(json.loads(data))
            except Exception:
                continue

    def timeseries(self, hours: int = 24) -> list[TimeseriesPoint]:
        """
        Return per-minute flow + byte counts for the last *hours* hours.

        :param hours: 1, 6, or 24 (default 24)
        """
        data = self._c._get("/api/v1/timeseries", params={"hours": hours})
        return [TimeseriesPoint.from_dict(p) for p in (data or [])]

    def top_talkers(
        self,
        *,
        window: str = "24h",
        by: str = "flows",
        limit: int = 10,
    ) -> list[dict[str, Any]]:
        """
        Return the top source IPs by flow count or bytes transferred.

        :param window: ``"1h"``, ``"6h"``, ``"24h"``, or ``"7d"``
        :param by:     ``"flows"`` or ``"bytes"``
        :param limit:  Number of results (default 10)
        :returns: List of dicts with ``src_ip``, ``flows``/``bytes``, ``pct``
        """
        return self._c._get(
            "/api/v1/flows/top-talkers",
            params={"window": window, "by": by, "limit": limit},
        ) or []
