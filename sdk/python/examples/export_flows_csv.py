"""
Example: Export flows to CSV for offline analysis or SIEM import.

Exports all TLS flows from the last 24 hours to a CSV file,
one row per flow with decoded TLS certificate information.

Run:
    python export_flows_csv.py --protocol TLS --hours 24 --out flows.csv
"""

import argparse
import csv
import os
import sys
from datetime import datetime, timezone

from netscope_sdk import NetScope

HUB_URL = os.environ.get("NETSCOPE_HUB", "http://localhost:8080")
TOKEN   = os.environ.get("NETSCOPE_TOKEN", "")


def main() -> None:
    parser = argparse.ArgumentParser(description="Export NetScope flows to CSV")
    parser.add_argument("--protocol", default="TLS", help="Protocol filter (default: TLS)")
    parser.add_argument("--hours",    type=int, default=24, help="Look-back window in hours")
    parser.add_argument("--out",      default="flows.csv", help="Output CSV filename")
    parser.add_argument("--limit",    type=int, default=50_000, help="Max flows to export")
    args = parser.parse_args()

    ns = NetScope(url=HUB_URL, token=TOKEN)
    if not ns.ping():
        print("ERROR: cannot reach Hub at", HUB_URL, file=sys.stderr)
        sys.exit(1)

    print(f"Exporting {args.protocol} flows from the last {args.hours}h → {args.out}")

    fieldnames = [
        "id", "timestamp", "src_ip", "src_port", "dst_ip", "dst_port",
        "protocol", "bytes_in", "bytes_out", "duration_ms",
        "country_code", "org", "threat_level",
        "process_name", "agent_id",
        # TLS-specific
        "tls_sni", "tls_version", "tls_cert_cn", "tls_cert_expiry",
        "tls_cert_expired", "tls_cert_issuer", "tls_has_weak_cipher",
    ]

    count = 0
    with open(args.out, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames, extrasaction="ignore")
        writer.writeheader()

        for flow in ns.flows.iter_all(
            protocol=args.protocol,
            hours=args.hours,
            page_size=500,
        ):
            row: dict = {
                "id": flow.id,
                "timestamp": flow.timestamp.isoformat() if flow.timestamp else "",
                "src_ip": flow.src_ip,
                "src_port": flow.src_port,
                "dst_ip": flow.dst_ip,
                "dst_port": flow.dst_port,
                "protocol": flow.protocol,
                "bytes_in": flow.bytes_in,
                "bytes_out": flow.bytes_out,
                "duration_ms": flow.duration_ms,
                "country_code": flow.country_code,
                "org": flow.org,
                "threat_level": flow.threat_level,
                "process_name": flow.process_name,
                "agent_id": flow.agent_id,
            }
            if flow.tls:
                row.update({
                    "tls_sni": flow.tls.sni,
                    "tls_version": flow.tls.negotiated_version or flow.tls.version,
                    "tls_cert_cn": flow.tls.cert_cn,
                    "tls_cert_expiry": flow.tls.cert_expiry,
                    "tls_cert_expired": flow.tls.cert_expired,
                    "tls_cert_issuer": flow.tls.cert_issuer,
                    "tls_has_weak_cipher": flow.tls.has_weak_cipher,
                })
            writer.writerow(row)
            count += 1
            if count >= args.limit:
                print(f"  Reached limit of {args.limit:,} flows, stopping.")
                break
            if count % 1000 == 0:
                print(f"  {count:,} flows written…")

    print(f"\nExported {count:,} flows to {args.out}")
    generated_at = datetime.now(timezone.utc).strftime("%Y-%m-%d %H:%M UTC")
    print(f"Generated at: {generated_at}")


if __name__ == "__main__":
    main()
