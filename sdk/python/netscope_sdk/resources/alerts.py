"""NetScope SDK — Alerts resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import AlertEvent, AlertRule

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class AlertsResource:
    """
    Manage alert rules and query alert events.

    Example::

        # Create a rule that fires on any high-threat outbound connection
        rule = client.alerts.create_rule(
            name="High-threat outbound",
            condition="threat_level = 'high'",
            severity="critical",
            integration="webhook",
            webhook_url="https://hooks.slack.com/services/xxx",
        )

        # List recent events
        events = client.alerts.list_events(severity="critical", limit=50)
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    # ── Rules ─────────────────────────────────────────────────────────────────

    def list_rules(self) -> list[AlertRule]:
        """Return all alert rules."""
        data = self._c._get("/api/v1/alerts/rules")
        return [AlertRule.from_dict(r) for r in (data or [])]

    def get_rule(self, rule_id: str) -> AlertRule:
        """Return a single alert rule by ID."""
        data = self._c._get(f"/api/v1/alerts/rules/{rule_id}")
        return AlertRule.from_dict(data)

    def create_rule(
        self,
        *,
        name: str,
        condition: str,
        severity: str = "medium",
        description: str = "",
        integration: str = "none",
        webhook_url: str = "",
        email_to: str = "",
        enabled: bool = True,
    ) -> AlertRule:
        """
        Create a new alert rule.

        :param name:        Human-readable rule name
        :param condition:   SQL-style WHERE condition over flow fields,
                            e.g. ``"threat_level = 'high' AND dst_port = 22"``
        :param severity:    ``"critical"``, ``"high"``, ``"medium"``, ``"low"``
        :param description: Optional description
        :param integration: ``"none"``, ``"webhook"``, or ``"email"``
        :param webhook_url: Webhook target URL (if integration is webhook)
        :param email_to:    Recipient email address (if integration is email)
        :param enabled:     Whether the rule is active immediately (default True)
        """
        payload: dict[str, Any] = {
            "name": name,
            "condition": condition,
            "severity": severity,
            "description": description,
            "integration": integration,
            "webhook_url": webhook_url,
            "email_to": email_to,
            "enabled": enabled,
        }
        data = self._c._post("/api/v1/alerts/rules", payload)
        return AlertRule.from_dict(data)

    def update_rule(self, rule_id: str, **kwargs: Any) -> AlertRule:
        """
        Update an existing alert rule.

        Pass only the fields you want to change as keyword arguments.
        """
        data = self._c._put(f"/api/v1/alerts/rules/{rule_id}", kwargs)
        return AlertRule.from_dict(data)

    def delete_rule(self, rule_id: str) -> None:
        """Delete an alert rule by ID."""
        self._c._delete(f"/api/v1/alerts/rules/{rule_id}")

    def test_rule(self, rule_id: str) -> dict[str, Any]:
        """
        Fire a test delivery for the rule immediately.

        :returns: Dict with ``"ok"`` bool and optional ``"error"`` string.
        """
        return self._c._post(f"/api/v1/alerts/{rule_id}/test", {}) or {}

    # ── Events ────────────────────────────────────────────────────────────────

    def list_events(
        self,
        *,
        severity: str | None = None,
        rule_id: str | None = None,
        hours: int | None = None,
        limit: int = 100,
    ) -> list[AlertEvent]:
        """
        Query alert events (fired alerts).

        :param severity: Filter by severity (``"critical"``, ``"high"``, etc.)
        :param rule_id:  Filter to a specific rule
        :param hours:    Look back N hours
        :param limit:    Max rows (default 100)
        """
        params: dict[str, Any] = {"limit": limit}
        if severity:
            params["severity"] = severity
        if rule_id:
            params["rule_id"] = rule_id
        if hours is not None:
            params["hours"] = hours

        data = self._c._get("/api/v1/alerts", params=params)
        return [AlertEvent.from_dict(e) for e in (data or [])]
