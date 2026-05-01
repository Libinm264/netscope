"""NetScope SDK — Agents resource."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from netscope_sdk.models import Agent

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


class AgentsResource:
    """
    Manage and monitor Capture Agents registered with this Hub.

    Example::

        # List all online agents
        online = [a for a in client.agents.list() if a.is_online]

        # Push a new capture filter remotely
        client.agents.push_config("agent-uuid", filter="not port 22")
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    def list(self) -> list[Agent]:
        """Return all registered agents."""
        data = self._c._get("/api/v1/agents")
        return [Agent.from_dict(a) for a in (data or [])]

    def get(self, agent_id: str) -> Agent:
        """Return a single agent by ID."""
        data = self._c._get(f"/api/v1/agents/{agent_id}")
        return Agent.from_dict(data)

    def delete(self, agent_id: str) -> None:
        """Decommission (soft-delete) an agent."""
        self._c._delete(f"/api/v1/agents/{agent_id}")

    def push_config(
        self,
        agent_id: str,
        *,
        filter: str | None = None,  # noqa: A002
        ifaces: list[str] | None = None,
        labels: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        """
        Push a configuration update to a remote agent.

        Changes are delivered over the agent's active WebSocket connection
        and applied within seconds — no SSH required.

        :param agent_id: Target agent UUID
        :param filter:   BPF filter expression, e.g. ``"not port 22"``
        :param ifaces:   New interface list, e.g. ``["eth0", "eth1"]``
        :param labels:   New label map, e.g. ``{"env": "staging"}``
        """
        payload: dict[str, Any] = {}
        if filter is not None:
            payload["filter"] = filter
        if ifaces is not None:
            payload["ifaces"] = ifaces
        if labels is not None:
            payload["labels"] = labels
        return self._c._put(f"/api/v1/agents/{agent_id}/config", payload) or {}

    def install_script(self, *, label: str = "", format: str = "sh") -> str:  # noqa: A002
        """
        Generate an enrollment install script for this Hub.

        :param label:  Optional label to pre-set on the agent
        :param format: ``"sh"`` (default) or ``"ps1"`` for PowerShell
        :returns: Shell script text as a string
        """
        params: dict[str, Any] = {"format": format}
        if label:
            params["label"] = label
        return self._c._get_raw("/api/v1/agents/install", params=params)
