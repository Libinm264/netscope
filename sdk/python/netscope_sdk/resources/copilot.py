"""NetScope SDK — AI Copilot resource."""

from __future__ import annotations

from collections.abc import Generator
from dataclasses import dataclass
from typing import TYPE_CHECKING, Any

if TYPE_CHECKING:
    from netscope_sdk.client import _BaseClient


@dataclass
class CopilotMessage:
    """A single message in a Copilot conversation."""
    role: str   # "user" or "assistant"
    content: str


@dataclass
class CopilotToken:
    """A streaming token chunk from the Copilot SSE response."""
    text: str
    sql: str | None = None          # set when the model emits a SQL query
    results: list[dict] | None = None  # set when a query result arrives


class CopilotResource:
    """
    Natural-language interface to your flow data via the AI Copilot.

    Requires ``ANTHROPIC_API_KEY`` to be set on the Hub.

    Example — one-shot question::

        answer = client.copilot.ask("Which host had the most outbound bytes yesterday?")
        print(answer)

    Example — streaming::

        for token in client.copilot.stream("Show DNS queries to .ru in the last hour"):
            print(token.text, end="", flush=True)

    Example — multi-turn conversation::

        chat = client.copilot.chat()
        print(chat.send("How many flows today?"))
        print(chat.send("Break that down by protocol"))
    """

    def __init__(self, client: "_BaseClient") -> None:
        self._c = client

    def ask(self, question: str, *, history: list[CopilotMessage] | None = None) -> str:
        """
        Send a question and return the complete assistant reply as a string.

        This buffers the full SSE stream and returns when the response is done.

        :param question: Natural-language question about your network data
        :param history:  Optional prior conversation turns for multi-turn context
        :returns: Full assistant response text
        """
        parts: list[str] = []
        for token in self.stream(question, history=history):
            parts.append(token.text)
        return "".join(parts)

    def stream(
        self,
        question: str,
        *,
        history: list[CopilotMessage] | None = None,
    ) -> Generator[CopilotToken, None, None]:
        """
        Stream the assistant response token by token.

        Yields :class:`CopilotToken` objects. When the model executes a SQL
        query, a token with ``sql`` set is yielded first, followed by a token
        with ``results`` set when the query returns, then the text narration.

        :param question: Natural-language question
        :param history:  Optional prior conversation turns
        """
        import json

        messages: list[dict[str, str]] = []
        for m in (history or []):
            messages.append({"role": m.role, "content": m.content})
        messages.append({"role": "user", "content": question})

        payload = {"messages": messages}

        for raw in self._c._sse("/api/v1/copilot/chat", body=payload):
            try:
                ev = json.loads(raw)
            except Exception:
                # plain text token
                yield CopilotToken(text=raw)
                continue

            ev_type = ev.get("type", "")
            if ev_type == "text":
                yield CopilotToken(text=ev.get("text", ""))
            elif ev_type == "query":
                yield CopilotToken(text="", sql=ev.get("sql", ""))
            elif ev_type == "result":
                yield CopilotToken(text="", results=ev.get("rows", []))
            elif ev_type == "error":
                from netscope_sdk.exceptions import NetScopeError
                raise NetScopeError(f"Copilot error: {ev.get('message', '')}")

    def chat(self) -> "_ConversationSession":
        """
        Return a stateful conversation session that accumulates history.

        Example::

            sess = client.copilot.chat()
            print(sess.send("What's the top talker today?"))
            print(sess.send("What ports does it use?"))  # remembers context
        """
        return _ConversationSession(self)


class _ConversationSession:
    """Stateful multi-turn Copilot session."""

    def __init__(self, copilot: CopilotResource) -> None:
        self._cop = copilot
        self._history: list[CopilotMessage] = []

    def send(self, message: str) -> str:
        """Send a message and return the full assistant reply."""
        reply = self._cop.ask(message, history=self._history)
        self._history.append(CopilotMessage(role="user", content=message))
        self._history.append(CopilotMessage(role="assistant", content=reply))
        return reply

    def reset(self) -> None:
        """Clear conversation history."""
        self._history = []

    @property
    def history(self) -> list[CopilotMessage]:
        """Read-only view of the conversation so far."""
        return list(self._history)
