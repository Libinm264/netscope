"""
netscope-sdk — Python SDK for the NetScope Hub API.

Usage::

    from netscope_sdk import NetScope

    ns = NetScope(url="https://hub.example.com", token="nst_xxx")
    print(ns.ping())

    flows = ns.flows.list(protocol="TLS", hours=1, limit=50)
    anomalies = ns.anomalies.list(severity="high")
    answer = ns.copilot.ask("Which host had the most outbound bytes today?")
"""

from netscope_sdk.client import NetScope
from netscope_sdk.exceptions import (
    AuthError,
    ConnectionError,
    ForbiddenError,
    NetScopeError,
    NotFoundError,
    RateLimitError,
    ServerError,
    ValidationError,
)
from netscope_sdk.models import (
    Agent,
    AlertEvent,
    AlertRule,
    Anomaly,
    Dashboard,
    Flow,
    Incident,
    SigmaRule,
    Stats,
    TimeseriesPoint,
    Widget,
)

__version__ = "0.6.0"
__author__ = "NetScope"
__all__ = [
    # Main client
    "NetScope",
    # Exceptions
    "NetScopeError",
    "AuthError",
    "ForbiddenError",
    "NotFoundError",
    "ValidationError",
    "RateLimitError",
    "ServerError",
    "ConnectionError",
    # Models
    "Flow",
    "AlertRule",
    "AlertEvent",
    "Anomaly",
    "Agent",
    "Dashboard",
    "Widget",
    "Incident",
    "SigmaRule",
    "Stats",
    "TimeseriesPoint",
]
