"""
Example: Threat Hunt — find and triage suspicious outbound connections.

This script queries the last hour for any flows to high-threat IPs,
groups them by source host, and auto-creates an incident for each
unique source that has more than 3 suspicious connections.

Run:
    pip install netscope-sdk
    python hunt_threats.py
"""

import os
from collections import defaultdict

from netscope_sdk import NetScope

HUB_URL = os.environ.get("NETSCOPE_HUB", "http://localhost:8080")
TOKEN   = os.environ.get("NETSCOPE_TOKEN", "")

ns = NetScope(url=HUB_URL, token=TOKEN)

print(f"Connected to {HUB_URL}  ✓" if ns.ping() else "ERROR: cannot reach hub")

# ── 1. Pull high-threat flows from the last hour ──────────────────────────────
print("\n[*] Querying high-threat flows (last 1 h)…")

flows = ns.flows.list(threat_level="high", hours=1, limit=1000)
print(f"    Found {len(flows)} high-threat flow(s)")

if not flows:
    print("No threats. All clear.")
    raise SystemExit(0)

# ── 2. Group by source IP ──────────────────────────────────────────────────────
by_src: dict[str, list] = defaultdict(list)
for f in flows:
    by_src[f.src_ip].append(f)

print("\n[*] Top offenders:")
for src, flist in sorted(by_src.items(), key=lambda x: -len(x[1]))[:5]:
    dsts = {f.dst_ip for f in flist}
    print(f"    {src:18s}  {len(flist):4d} flows  →  {len(dsts)} unique destinations")

# ── 3. Auto-create incidents for hosts with ≥ 3 hits ─────────────────────────
THRESHOLD = 3
print(f"\n[*] Creating incidents for hosts with ≥ {THRESHOLD} hits…")

for src, flist in by_src.items():
    if len(flist) < THRESHOLD:
        continue

    dsts = sorted({f.dst_ip for f in flist})[:5]
    protocols = sorted({f.protocol for f in flist})
    total_bytes = sum(f.total_bytes for f in flist)

    title = f"High-threat outbound from {src} ({len(flist)} flows)"
    description = (
        f"Host {src} made {len(flist)} connections to high-threat IPs "
        f"in the last hour.\n\n"
        f"Destination IPs (sample): {', '.join(dsts)}\n"
        f"Protocols: {', '.join(protocols)}\n"
        f"Total bytes: {total_bytes:,}\n"
    )

    severity = "P1" if len(flist) >= 20 else "P2" if len(flist) >= 10 else "P3"

    inc = ns.incidents.create(
        title=title,
        severity=severity,
        description=description,
    )
    print(f"    [{severity}] Created incident {inc.id} — {title}")

print("\nDone.")
