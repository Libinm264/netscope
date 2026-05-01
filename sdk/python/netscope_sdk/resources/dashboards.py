"""NetScope SDK — Dashboards resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import Dashboard, Widget

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class DashboardsResource:
    """
    Create and manage Custom Dashboards.

    Example::

        dash = client.dashboards.create(
            name="Security Overview",
            description="Threats and anomalies at a glance",
            widgets=[
                {"type": "stat",       "title": "Alerts (24h)", "size": "sm",
                 "config": {"metric": "alert_count"}},
                {"type": "timeseries", "title": "Flow Volume",   "size": "lg",
                 "config": {"window": "24h"}},
            ],
        )
        print("Created:", dash.id)
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    def list(self) -> list[Dashboard]:
        """Return all dashboards for the current organisation."""
        data = self._c._get("/api/v1/dashboards")
        return [Dashboard.from_dict(d) for d in (data or [])]

    def get(self, dashboard_id: str) -> Dashboard:
        """Return a single dashboard by ID."""
        data = self._c._get(f"/api/v1/dashboards/{dashboard_id}")
        return Dashboard.from_dict(data)

    def create(
        self,
        *,
        name: str,
        description: str = "",
        widgets: list[dict[str, Any]] | None = None,
    ) -> Dashboard:
        """
        Create a new dashboard.

        :param name:        Dashboard name (required)
        :param description: Optional description
        :param widgets:     Optional list of widget dicts.  Each dict must have
                            ``type``, ``title``, ``size``, and ``config`` keys.
                            Supported types: ``stat``, ``timeseries``,
                            ``protocol_pie``, ``top_talkers``, ``alert_feed``,
                            ``anomaly_feed``.
        """
        payload: dict[str, Any] = {
            "name": name,
            "description": description,
            "widgets": widgets or [],
        }
        data = self._c._post("/api/v1/dashboards", payload)
        return Dashboard.from_dict(data)

    def update(
        self,
        dashboard_id: str,
        *,
        name: str | None = None,
        description: str | None = None,
        widgets: list[dict[str, Any]] | None = None,
    ) -> Dashboard:
        """
        Update a dashboard's name, description, or widgets.

        Only the fields you supply will be changed.
        """
        payload: dict[str, Any] = {}
        if name is not None:
            payload["name"] = name
        if description is not None:
            payload["description"] = description
        if widgets is not None:
            payload["widgets"] = widgets
        data = self._c._put(f"/api/v1/dashboards/{dashboard_id}", payload)
        return Dashboard.from_dict(data)

    def delete(self, dashboard_id: str) -> None:
        """Delete a dashboard by ID."""
        self._c._delete(f"/api/v1/dashboards/{dashboard_id}")

    # ── Widget helpers ────────────────────────────────────────────────────────

    @staticmethod
    def stat_widget(
        title: str,
        metric: str,
        size: str = "sm",
    ) -> dict[str, Any]:
        """
        Build a stat widget dict.

        :param metric: One of ``total_flows``, ``active_agents``, ``total_bytes``,
                       ``anomaly_count``, ``alert_count``
        """
        return {"type": "stat", "title": title, "size": size,
                "config": {"metric": metric}}

    @staticmethod
    def timeseries_widget(
        title: str = "Flow Volume",
        window: str = "24h",
        size: str = "lg",
    ) -> dict[str, Any]:
        """Build a time-series line-chart widget dict."""
        return {"type": "timeseries", "title": title, "size": size,
                "config": {"window": window}}

    @staticmethod
    def top_talkers_widget(
        title: str = "Top Talkers",
        window: str = "24h",
        by: str = "flows",
        limit: int = 10,
        size: str = "md",
    ) -> dict[str, Any]:
        """Build a top-talkers bar widget dict."""
        return {"type": "top_talkers", "title": title, "size": size,
                "config": {"window": window, "by": by, "limit": limit}}

    @staticmethod
    def alert_feed_widget(
        title: str = "Recent Alerts",
        limit: int = 8,
        size: str = "md",
    ) -> dict[str, Any]:
        """Build an alert-feed widget dict."""
        return {"type": "alert_feed", "title": title, "size": size,
                "config": {"limit": limit}}

    @staticmethod
    def anomaly_feed_widget(
        title: str = "Recent Anomalies",
        limit: int = 8,
        severity: str = "all",
        size: str = "md",
    ) -> dict[str, Any]:
        """Build an anomaly-feed widget dict."""
        return {"type": "anomaly_feed", "title": title, "size": size,
                "config": {"limit": limit, "severity": severity}}
