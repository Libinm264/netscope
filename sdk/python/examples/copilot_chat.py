"""
Example: Interactive AI Copilot session in the terminal.

Start a multi-turn conversation with the NetScope AI Copilot.
Ask questions about your network data in plain English.

Run:
    python copilot_chat.py
"""

import os
import sys

from netscope_sdk import NetScope

HUB_URL = os.environ.get("NETSCOPE_HUB", "http://localhost:8080")
TOKEN   = os.environ.get("NETSCOPE_TOKEN", "")

BANNER = """
╔═══════════════════════════════════════════════╗
║      NetScope AI Copilot  (SDK example)       ║
║  Type your question, or 'exit' to quit.       ║
╚═══════════════════════════════════════════════╝
"""

SUGGESTIONS = [
    "  • Which host had the most outbound bytes today?",
    "  • Show me all DNS queries to .ru domains in the last hour",
    "  • How many TLS connections used weak ciphers yesterday?",
    "  • Are there any anomalies in the last 30 minutes?",
    "  • Write a Sigma rule to detect port scanning",
]


def main() -> None:
    ns = NetScope(url=HUB_URL, token=TOKEN)
    if not ns.ping():
        print(f"ERROR: cannot reach Hub at {HUB_URL}", file=sys.stderr)
        sys.exit(1)

    print(BANNER)
    print("Suggestions:")
    print("\n".join(SUGGESTIONS))
    print()

    session = ns.copilot.chat()

    while True:
        try:
            question = input("You: ").strip()
        except (EOFError, KeyboardInterrupt):
            print("\nBye!")
            break

        if not question:
            continue
        if question.lower() in {"exit", "quit", "q"}:
            print("Bye!")
            break

        print("\nCopilot: ", end="", flush=True)
        try:
            for token in ns.copilot.stream(question, history=session.history):
                if token.sql:
                    print(f"\n  [SQL] {token.sql}", flush=True)
                elif token.results is not None:
                    print(f"\n  [{len(token.results)} rows returned]", flush=True)
                elif token.text:
                    print(token.text, end="", flush=True)

            # Update session history for multi-turn context
            # (re-use session.send for next turn to get full reply buffered)
        except Exception as exc:
            print(f"\n[error] {exc}")

        print("\n")


if __name__ == "__main__":
    main()
