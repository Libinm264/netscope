"""NetScope SDK — HTTP client base and public NetScope entry point."""

from __future__ import annotations

import json as _json
import urllib.error
import urllib.parse
import urllib.request
from collections.abc import Generator
from typing import Any

from netscope_sdk.exceptions import ConnectionError, _raise_for_status  # noqa: A004
from netscope_sdk.models import Stats, TimeseriesPoint
from netscope_sdk.resources import (
    AgentsResource,
    AlertsResource,
    AnomaliesResource,
    CopilotResource,
    DashboardsResource,
    FlowsResource,
    IncidentsResource,
    SigmaResource,
)

__all__ = ["NetScope"]

_DEFAULT_TIMEOUT = 30  # seconds


class _BaseClient:
    """
    Thin HTTP wrapper around the NetScope Hub REST API.

    Uses the stdlib ``urllib`` so there are **zero required dependencies**.
    Install ``requests`` or ``httpx`` only if you prefer them — but the
    default implementation works out of the box.
    """

    def __init__(
        self,
        url: str,
        token: str,
        *,
        timeout: int = _DEFAULT_TIMEOUT,
        verify_ssl: bool = True,
    ) -> None:
        self._base = url.rstrip("/")
        self._token = token
        self._timeout = timeout
        self._verify_ssl = verify_ssl

        if not verify_ssl:
            import ssl
            self._ssl_ctx: Any = ssl.create_default_context()
            self._ssl_ctx.check_hostname = False
            self._ssl_ctx.verify_mode = ssl.CERT_NONE
        else:
            self._ssl_ctx = None

    def _headers(self) -> dict[str, str]:
        return {
            "Authorization": f"Bearer {self._token}",
            "Content-Type": "application/json",
            "Accept": "application/json",
            "User-Agent": "netscope-sdk-python/0.6.0",
        }

    def _url(self, path: str, params: dict[str, Any] | None = None) -> str:
        full = self._base + path
        if params:
            query = urllib.parse.urlencode(
                {k: v for k, v in params.items() if v is not None}
            )
            full = f"{full}?{query}"
        return full

    def _request(
        self,
        method: str,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        body: Any | None = None,
    ) -> Any:
        url = self._url(path, params)
        data = _json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, headers=self._headers(), method=method)
        try:
            kwargs: dict[str, Any] = {"timeout": self._timeout}
            if self._ssl_ctx:
                kwargs["context"] = self._ssl_ctx
            with urllib.request.urlopen(req, **kwargs) as resp:
                raw = resp.read().decode()
                if not raw.strip():
                    return None
                return _json.loads(raw)
        except urllib.error.HTTPError as exc:
            body_text = exc.read().decode(errors="replace")
            _raise_for_status(exc.code, body_text)
        except OSError as exc:
            raise ConnectionError(f"Cannot reach Hub at {self._base}: {exc}") from exc

    def _get(self, path: str, *, params: dict[str, Any] | None = None) -> Any:
        return self._request("GET", path, params=params)

    def _get_raw(self, path: str, *, params: dict[str, Any] | None = None) -> str:
        url = self._url(path, params)
        req = urllib.request.Request(url, headers=self._headers(), method="GET")
        try:
            kwargs: dict[str, Any] = {"timeout": self._timeout}
            if self._ssl_ctx:
                kwargs["context"] = self._ssl_ctx
            with urllib.request.urlopen(req, **kwargs) as resp:
                return resp.read().decode()
        except urllib.error.HTTPError as exc:
            body_text = exc.read().decode(errors="replace")
            _raise_for_status(exc.code, body_text)
        except OSError as exc:
            raise ConnectionError(f"Cannot reach Hub at {self._base}: {exc}") from exc
        return ""  # unreachable

    def _post(self, path: str, body: Any) -> Any:
        return self._request("POST", path, body=body)

    def _put(self, path: str, body: Any) -> Any:
        return self._request("PUT", path, body=body)

    def _delete(self, path: str) -> None:
        self._request("DELETE", path)

    def _sse(
        self,
        path: str,
        *,
        params: dict[str, Any] | None = None,
        body: Any | None = None,
    ) -> Generator[str, None, None]:
        """
        Consume a Server-Sent Events endpoint.

        Yields the ``data:`` payload of each SSE event as a raw string.
        Stops when the connection closes or a ``[DONE]`` sentinel is received.
        """
        url = self._url(path, params)
        headers = {**self._headers(), "Accept": "text/event-stream"}
        method = "POST" if body is not None else "GET"
        data = _json.dumps(body).encode() if body is not None else None
        req = urllib.request.Request(url, data=data, headers=headers, method=method)

        try:
            kwargs: dict[str, Any] = {"timeout": self._timeout}
            if self._ssl_ctx:
                kwargs["context"] = self._ssl_ctx
            with urllib.request.urlopen(req, **kwargs) as resp:
                for raw_line in resp:
                    line = raw_line.decode(errors="replace").rstrip("\r\n")
                    if line.startswith("data:"):
                        payload = line[5:].strip()
                        if payload == "[DONE]":
                            return
                        yield payload
        except urllib.error.HTTPError as exc:
            body_text = exc.read().decode(errors="replace")
            _raise_for_status(exc.code, body_text)
        except OSError as exc:
            raise ConnectionError(f"SSE stream error at {self._base}: {exc}") from exc


# ── Public entry point ────────────────────────────────────────────────────────

class NetScope(_BaseClient):
    """
    The main entry point for the NetScope Python SDK.

    Instantiate once and reuse across your application.

    **Quick start**::

        from netscope_sdk import NetScope

        ns = NetScope(
            url="https://hub.example.com",
            token="nst_xxxxxxxxxxxxxxxxxxxx",
        )

        # Hub health
        print(ns.ping())

        # Recent high-threat flows
        for flow in ns.flows.list(threat_level="high", hours=1):
            print(flow.src_ip, "→", flow.dst_ip, flow.protocol)

        # Summary stats
        stats = ns.stats()
        print(f"{stats.total_flows:,} total flows, {stats.active_agents} agents online")

    **Available resources**

    ===============  ======================================================
    ``ns.flows``     Query, stream, and paginate network flows
    ``ns.alerts``    Manage alert rules and read alert events
    ``ns.anomalies`` Query anomaly detection events
    ``ns.agents``    List agents, push remote configuration
    ``ns.dashboards`` Create and manage custom dashboards
    ``ns.incidents`` Manage the incident lifecycle
    ``ns.sigma``     Import and manage Sigma detection rules
    ``ns.copilot``   Natural-language interface to your flow data (AI)
    ===============  ======================================================
    """

    def __init__(
        self,
        url: str,
        token: str,
        *,
        timeout: int = _DEFAULT_TIMEOUT,
        verify_ssl: bool = True,
    ) -> None:
        """
        Create a NetScope client.

        :param url:        Full URL of your Hub, e.g. ``https://hub.example.com``
        :param token:      API token generated in Hub → Settings → Tokens
        :param timeout:    Request timeout in seconds (default 30)
        :param verify_ssl: Set ``False`` to disable TLS verification (not recommended)
        """
        super().__init__(url, token, timeout=timeout, verify_ssl=verify_ssl)

        # Resource accessors
        self.flows = FlowsResource(self)
        self.alerts = AlertsResource(self)
        self.anomalies = AnomaliesResource(self)
        self.agents = AgentsResource(self)
        self.dashboards = DashboardsResource(self)
        self.incidents = IncidentsResource(self)
        self.sigma = SigmaResource(self)
        self.copilot = CopilotResource(self)

    # ── Top-level helpers ─────────────────────────────────────────────────────

    def ping(self) -> bool:
        """
        Check Hub reachability and token validity.

        :returns: ``True`` if the Hub responds with a valid 200, ``False`` otherwise.
        """
        try:
            self._get("/api/v1/stats")
            return True
        except Exception:
            return False

    def stats(self) -> Stats:
        """
        Return aggregate statistics for the Hub (flows, agents, anomalies…).

        Example::

            s = ns.stats()
            print(f"Flows/min: {s.flows_per_minute:.1f}")
            print(f"Anomalies (24h): {s.anomaly_count_24h}")
        """
        data = self._get("/api/v1/stats")
        return Stats.from_dict(data or {})

    def timeseries(self, hours: int = 24) -> list[TimeseriesPoint]:
        """Shortcut for :meth:`flows.timeseries`."""
        return self.flows.timeseries(hours=hours)
