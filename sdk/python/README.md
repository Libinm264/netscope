# netscope-sdk

Official Python SDK for the [NetScope](https://netscope.io) Hub API.

**Zero required dependencies** — uses Python's stdlib `urllib` by default.
Works out of the box on Python 3.9+.

---

## Installation

```bash
pip install netscope-sdk
```

---

## Quick start

```python
from netscope_sdk import NetScope

ns = NetScope(
    url="https://hub.example.com",
    token="nst_xxxxxxxxxxxxxxxxxxxx",   # Hub → Settings → Tokens
)

# Verify connection
assert ns.ping(), "Cannot reach hub"

# Aggregate stats
stats = ns.stats()
print(f"{stats.total_flows:,} total flows, {stats.active_agents} agents online")

# Recent high-threat flows
for flow in ns.flows.list(threat_level="high", hours=1):
    print(f"{flow.src_ip} → {flow.dst_ip}  [{flow.threat_level}]  {flow.protocol}")

# Ask the AI Copilot a question
answer = ns.copilot.ask("Which host had the most outbound bytes yesterday?")
print(answer)
```

---

## Resources

| Accessor | What it does |
|---|---|
| `ns.flows` | Query, stream, and paginate network flows |
| `ns.alerts` | Manage alert rules and read alert events |
| `ns.anomalies` | Query anomaly detection events |
| `ns.agents` | List agents, push remote configuration |
| `ns.dashboards` | Create and manage custom dashboards |
| `ns.incidents` | Manage the incident lifecycle |
| `ns.sigma` | Import and manage Sigma detection rules |
| `ns.copilot` | Natural-language interface to your flow data (AI) |

---

## Flows

```python
# Basic query with filters
flows = ns.flows.list(
    protocol="TLS",
    country="CN",
    hours=24,
    limit=500,
)

# Auto-paginating iterator (fetches all pages automatically)
for flow in ns.flows.iter_all(threat_level="high", hours=72):
    process(flow)

# Real-time SSE stream (blocks until disconnected)
for flow in ns.flows.stream():
    if flow.is_threat:
        alert(flow)

# Time-series data (flows per minute over last 24h)
series = ns.flows.timeseries(hours=24)
for point in series:
    print(point.ts, point.flows, point.bytes_in + point.bytes_out)

# Top talkers
talkers = ns.flows.top_talkers(window="1h", by="bytes", limit=10)
```

### Flow fields

```python
flow.id, flow.timestamp, flow.agent_id, flow.hostname
flow.src_ip, flow.src_port, flow.dst_ip, flow.dst_port
flow.protocol          # "TLS", "DNS", "HTTP", "HTTP2", "GRPC", etc.
flow.bytes_in, flow.bytes_out, flow.total_bytes
flow.duration_ms
flow.country_code, flow.country_name, flow.asn, flow.org
flow.threat_score, flow.threat_level   # "high" | "medium" | "low" | ""
flow.process_name, flow.pid            # eBPF mode only
flow.tls, flow.dns, flow.http          # protocol-specific sub-objects, or None
flow.is_threat                         # True for high/medium threat
```

---

## Alerts

```python
# List all rules
rules = ns.alerts.list_rules()

# Create a new rule
rule = ns.alerts.create_rule(
    name="Exfil to China",
    condition="country_code = 'CN' AND bytes_out > 1000000",
    severity="high",
    integration="webhook",
    webhook_url="https://hooks.slack.com/services/…",
)

# Test a rule's delivery
ns.alerts.test_rule(rule.id)

# Query fired events
events = ns.alerts.list_events(severity="critical", hours=24)
```

---

## Anomalies

```python
highs = ns.anomalies.list(severity="high", hours=6)
stats  = ns.anomalies.stats()   # {"high": 3, "medium": 12, "low": 45, "total": 60}
```

---

## Agents

```python
agents = ns.agents.list()
online = [a for a in agents if a.is_online]

# Push a new capture filter remotely (no SSH needed)
ns.agents.push_config(
    agent_id="uuid",
    filter="not port 22",
    labels={"env": "production"},
)
```

---

## Dashboards

```python
from netscope_sdk.resources.dashboards import DashboardsResource as D

dash = ns.dashboards.create(
    name="Security Overview",
    widgets=[
        D.stat_widget("Alerts (24h)", metric="alert_count", size="sm"),
        D.stat_widget("Anomalies",    metric="anomaly_count", size="sm"),
        D.timeseries_widget("Flow Volume", window="24h", size="lg"),
        D.top_talkers_widget(by="bytes", limit=10, size="md"),
        D.alert_feed_widget(limit=8, size="md"),
    ],
)
print(f"Dashboard URL: {HUB_URL}/dashboards/{dash.id}")
```

---

## Incidents

```python
inc = ns.incidents.create(
    title="Possible C2 beaconing",
    severity="P2",
    description="Host 10.0.0.5 contacted 3 known C2 IPs in 5 minutes",
)

ns.incidents.acknowledge(inc.id)
ns.incidents.add_note(inc.id, "Confirmed — isolating host")
ns.incidents.resolve(inc.id, "Host isolated, threat removed")
```

---

## Sigma rules

```python
# List all rules
rules = ns.sigma.list_rules(enabled=True)

# Import a custom rule (Enterprise required)
rule = ns.sigma.create_rule(
    title="Suspicious DNS Tunnelling",
    rule_yaml=open("dns_tunnel.yml").read(),
    level="high",
)

# Query recent matches
matches = ns.sigma.list_matches(rule_id=rule.id, hours=24)
```

---

## AI Copilot

```python
# One-shot question
answer = ns.copilot.ask("Show me all DNS queries to .ru domains today")
print(answer)

# Streaming (token by token)
for token in ns.copilot.stream("Which hosts are talking to Tor exit nodes?"):
    print(token.text, end="", flush=True)

# Multi-turn conversation
chat = ns.copilot.chat()
print(chat.send("What's our top talker today?"))
print(chat.send("What ports does it use?"))   # remembers context
```

---

## Error handling

```python
from netscope_sdk import NetScopeError, AuthError, NotFoundError, RateLimitError

try:
    flows = ns.flows.list(...)
except AuthError:
    print("Invalid or expired token")
except RateLimitError:
    print("Back off and retry")
except NetScopeError as e:
    print(f"HTTP {e.status_code}: {e}")
```

---

## Examples

| Script | What it does |
|---|---|
| `examples/hunt_threats.py` | Find threat flows, group by host, auto-create incidents |
| `examples/export_flows_csv.py` | Export all TLS flows to CSV with cert metadata |
| `examples/copilot_chat.py` | Interactive terminal AI Copilot session |
| `examples/build_dashboard.py` | Programmatically create a security overview dashboard |

---

## Development

```bash
git clone https://github.com/Libinm264/netscope
cd netscope/sdk/python
pip install -e ".[dev]"
pytest
```

---

## License

MIT © NetScope
