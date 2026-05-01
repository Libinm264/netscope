"""NetScope SDK — Incidents resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import Incident

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class IncidentsResource:
    """
    Manage the incident lifecycle: create, triage, resolve.

    Example::

        inc = client.incidents.create(
            title="Possible C2 beaconing to 185.220.x.x",
            severity="P2",
            description="Three hosts contacted same suspicious IP in 5 min window",
        )
        client.incidents.add_note(inc.id, "Confirmed — isolating host 10.0.1.55")
        client.incidents.resolve(inc.id, "Host isolated, threat remediated")
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    def list(
        self,
        *,
        status: str | None = None,
        severity: str | None = None,
        limit: int = 50,
        offset: int = 0,
    ) -> list[Incident]:
        """
        Return incidents matching the given filters.

        :param status:   ``"open"``, ``"in_progress"``, or ``"resolved"``
        :param severity: ``"P1"``, ``"P2"``, ``"P3"``, or ``"P4"``
        :param limit:    Max rows (default 50)
        :param offset:   Pagination offset
        """
        params: dict[str, Any] = {"limit": limit, "offset": offset}
        if status:
            params["status"] = status
        if severity:
            params["severity"] = severity
        data = self._c._get("/api/v1/incidents", params=params)
        return [Incident.from_dict(i) for i in (data or [])]

    def get(self, incident_id: str) -> Incident:
        """Return a single incident by ID."""
        data = self._c._get(f"/api/v1/incidents/{incident_id}")
        return Incident.from_dict(data)

    def create(
        self,
        *,
        title: str,
        severity: str = "P3",
        description: str = "",
        assignee: str = "",
    ) -> Incident:
        """
        Open a new incident.

        :param title:       Short description of the incident
        :param severity:    ``"P1"`` (critical) through ``"P4"`` (informational)
        :param description: Longer initial notes
        :param assignee:    Email of the person responsible
        """
        payload: dict[str, Any] = {
            "title": title,
            "severity": severity,
            "description": description,
            "assignee": assignee,
            "status": "open",
        }
        data = self._c._post("/api/v1/incidents", payload)
        return Incident.from_dict(data)

    def update(self, incident_id: str, **kwargs: Any) -> Incident:
        """Update any incident field (status, severity, assignee, notes…)."""
        data = self._c._put(f"/api/v1/incidents/{incident_id}", kwargs)
        return Incident.from_dict(data)

    def add_note(self, incident_id: str, note: str) -> Incident:
        """Append an investigation note to the incident."""
        return self.update(incident_id, notes=note)

    def acknowledge(self, incident_id: str) -> Incident:
        """Move incident to *in_progress*."""
        return self.update(incident_id, status="in_progress")

    def resolve(self, incident_id: str, resolution: str = "") -> Incident:
        """Close the incident with an optional resolution summary."""
        return self.update(incident_id, status="resolved", notes=resolution)

    def delete(self, incident_id: str) -> None:
        """Permanently delete an incident record."""
        self._c._delete(f"/api/v1/incidents/{incident_id}")
