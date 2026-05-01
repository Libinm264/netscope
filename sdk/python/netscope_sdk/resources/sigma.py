"""NetScope SDK — Sigma detection rules resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import SigmaRule

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class SigmaResource:
    """
    Manage Sigma detection rules and query match history.

    Requires an Enterprise license for creating custom rules.
    Built-in rules are readable on all tiers.

    Example::

        # List all enabled rules
        rules = client.sigma.list_rules(enabled=True)

        # Import a Sigma YAML rule
        rule = client.sigma.create_rule(
            title="Port Scan Detection",
            level="high",
            rule_yaml=open("portscan.yml").read(),
        )

        # Check for recent matches
        matches = client.sigma.list_matches(rule_id=rule.id, hours=1)
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    # ── Rules ─────────────────────────────────────────────────────────────────

    def list_rules(self, *, enabled: bool | None = None) -> list[SigmaRule]:
        """
        Return all Sigma rules.

        :param enabled: If set, filter to only enabled (``True``) or only
                        disabled (``False``) rules.
        """
        params: dict[str, Any] = {}
        if enabled is not None:
            params["enabled"] = str(enabled).lower()
        data = self._c._get("/api/v1/sigma", params=params)
        return [SigmaRule.from_dict(r) for r in (data or [])]

    def get_rule(self, rule_id: str) -> SigmaRule:
        """Return a single Sigma rule by ID."""
        data = self._c._get(f"/api/v1/sigma/{rule_id}")
        return SigmaRule.from_dict(data)

    def create_rule(
        self,
        *,
        title: str,
        rule_yaml: str,
        level: str = "medium",
        description: str = "",
        enabled: bool = True,
    ) -> SigmaRule:
        """
        Import a new Sigma rule. *Enterprise tier required.*

        :param title:       Human-readable rule title
        :param rule_yaml:   Full Sigma YAML source
        :param level:       ``"informational"``, ``"low"``, ``"medium"``,
                            ``"high"``, or ``"critical"``
        :param description: Optional description (may also come from the YAML)
        :param enabled:     Activate the rule immediately (default True)
        """
        payload: dict[str, Any] = {
            "title": title,
            "rule_yaml": rule_yaml,
            "level": level,
            "description": description,
            "enabled": enabled,
        }
        data = self._c._post("/api/v1/sigma", payload)
        return SigmaRule.from_dict(data)

    def update_rule(self, rule_id: str, **kwargs: Any) -> SigmaRule:
        """Update a Sigma rule. Accepts ``enabled``, ``level``, ``rule_yaml`` etc."""
        data = self._c._put(f"/api/v1/sigma/{rule_id}", kwargs)
        return SigmaRule.from_dict(data)

    def enable(self, rule_id: str) -> SigmaRule:
        """Enable a previously disabled rule."""
        return self.update_rule(rule_id, enabled=True)

    def disable(self, rule_id: str) -> SigmaRule:
        """Disable a rule without deleting it."""
        return self.update_rule(rule_id, enabled=False)

    def delete_rule(self, rule_id: str) -> None:
        """Delete a custom Sigma rule. Built-in rules cannot be deleted."""
        self._c._delete(f"/api/v1/sigma/{rule_id}")

    # ── Matches ───────────────────────────────────────────────────────────────

    def list_matches(
        self,
        *,
        rule_id: str | None = None,
        level: str | None = None,
        hours: int | None = None,
        limit: int = 100,
    ) -> list[dict[str, Any]]:
        """
        Return Sigma rule match events.

        :param rule_id: Filter to matches from a specific rule
        :param level:   Filter by severity level
        :param hours:   Look-back window in hours
        :param limit:   Max rows (default 100)
        """
        params: dict[str, Any] = {"limit": limit}
        if rule_id:
            params["rule_id"] = rule_id
        if level:
            params["level"] = level
        if hours is not None:
            params["hours"] = hours
        return self._c._get("/api/v1/sigma/matches", params=params) or []
