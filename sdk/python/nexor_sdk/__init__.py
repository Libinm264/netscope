"""
nexor-sdk — Python SDK for the Nexor Hub API.

Usage::

    from nexor_sdk import Nexor

    ns = Nexor(url="https://hub.example.com", token="nst_xxx")
    print(ns.ping())

    flows = ns.flows.list(protocol="TLS", hours=1, limit=50)
    anomalies = ns.anomalies.list(severity="high")
    answer = ns.copilot.ask("Which host had the most outbound bytes today?")
"""

from nexor_sdk.client import Nexor
from nexor_sdk.exceptions import (
    AuthError,
    ConnectionError,
    ForbiddenError,
    NexorError,
    NotFoundError,
    RateLimitError,
    ServerError,
    ValidationError,
)
from nexor_sdk.models import (
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
__author__ = "Nexor"
__all__ = [
    # Main client
    "Nexor",
    # Exceptions
    "NexorError",
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
