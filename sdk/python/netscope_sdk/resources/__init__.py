"""NetScope SDK — resource modules."""

from .flows import FlowsResource
from .alerts import AlertsResource
from .anomalies import AnomaliesResource
from .agents import AgentsResource
from .dashboards import DashboardsResource
from .incidents import IncidentsResource
from .sigma import SigmaResource
from .copilot import CopilotResource

__all__ = [
    "FlowsResource",
    "AlertsResource",
    "AnomaliesResource",
    "AgentsResource",
    "DashboardsResource",
    "IncidentsResource",
    "SigmaResource",
    "CopilotResource",
]
