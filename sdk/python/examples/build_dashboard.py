"""
Example: Programmatically create a "Security Overview" dashboard.

Creates a dashboard with 6 widgets covering the most important
security metrics: threat alerts, anomalies, flow volume, top talkers,
protocol breakdown, and an alert feed.

Run:
    python build_dashboard.py
"""

import os
import sys

from netscope_sdk import NetScope
from netscope_sdk.resources.dashboards import DashboardsResource

HUB_URL = os.environ.get("NETSCOPE_HUB", "http://localhost:8080")
TOKEN   = os.environ.get("NETSCOPE_TOKEN", "")


def main() -> None:
    ns = NetScope(url=HUB_URL, token=TOKEN)
    if not ns.ping():
        print(f"ERROR: cannot reach Hub at {HUB_URL}", file=sys.stderr)
        sys.exit(1)

    D = DashboardsResource  # just for the static widget helpers

    widgets = [
        # Row 1: three stat tiles
        D.stat_widget("Alert Count (24h)",    metric="alert_count",    size="sm"),
        D.stat_widget("Anomaly Count (24h)",  metric="anomaly_count",  size="sm"),
        D.stat_widget("Active Agents",        metric="active_agents",  size="sm"),
        # Row 2: full-width time series
        D.timeseries_widget("Flow Volume — Last 24h", window="24h", size="lg"),
        # Row 3: two half-width widgets
        D.top_talkers_widget("Top Talkers", window="24h", by="bytes", limit=10, size="md"),
        D.alert_feed_widget("Recent Critical Alerts", limit=8, size="md"),
        # Row 4: anomaly feed
        D.anomaly_feed_widget("High Anomalies", limit=8, severity="high", size="lg"),
    ]

    # Check if a dashboard with this name already exists
    existing = ns.dashboards.list()
    for d in existing:
        if d.name == "Security Overview":
            print(f"Updating existing dashboard {d.id}…")
            updated = ns.dashboards.update(
                d.id, description="Auto-generated security overview", widgets=widgets
            )
            print(f"Updated: {updated.name} ({len(updated.widgets)} widgets)")
            print(f"URL: {HUB_URL}/dashboards/{updated.id}")
            return

    # Create new
    dash = ns.dashboards.create(
        name="Security Overview",
        description="Auto-generated security overview: threats, anomalies, flow volume",
        widgets=widgets,
    )
    print(f"Created: {dash.name} ({len(dash.widgets)} widgets)")
    print(f"URL: {HUB_URL}/dashboards/{dash.id}")


if __name__ == "__main__":
    main()
