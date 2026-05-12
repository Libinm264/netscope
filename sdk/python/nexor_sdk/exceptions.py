"""Klyzar SDK — exception hierarchy."""

from __future__ import annotations


class NexorError(Exception):
    """Base exception for all Klyzar SDK errors."""

    def __init__(self, message: str, status_code: int | None = None, response_body: str | None = None) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.response_body = response_body

    def __repr__(self) -> str:
        parts = [f"NexorError({self.args[0]!r}"]
        if self.status_code is not None:
            parts.append(f", status_code={self.status_code}")
        return "".join(parts) + ")"


class AuthError(NexorError):
    """Raised when the API returns 401 Unauthorized."""


class ForbiddenError(NexorError):
    """Raised when the API returns 403 Forbidden (insufficient role/license)."""


class NotFoundError(NexorError):
    """Raised when the API returns 404 Not Found."""


class ValidationError(NexorError):
    """Raised when the API returns 400 Bad Request (invalid parameters)."""


class RateLimitError(NexorError):
    """Raised when the API returns 429 Too Many Requests."""


class ServerError(NexorError):
    """Raised when the API returns 5xx."""


class ConnectionError(NexorError):  # noqa: A001
    """Raised when the SDK cannot reach the Hub (network error)."""


def _raise_for_status(status_code: int, body: str) -> None:
    """Map an HTTP status code to the appropriate exception."""
    if status_code == 400:
        raise ValidationError(f"Bad request: {body}", status_code, body)
    if status_code == 401:
        raise AuthError("Unauthorized — check your API token", status_code, body)
    if status_code == 403:
        raise ForbiddenError("Forbidden — insufficient permissions or license tier", status_code, body)
    if status_code == 404:
        raise NotFoundError(f"Not found: {body}", status_code, body)
    if status_code == 429:
        raise RateLimitError("Rate limit exceeded — back off and retry", status_code, body)
    if status_code >= 500:
        raise ServerError(f"Hub server error ({status_code}): {body}", status_code, body)
    if status_code >= 400:
        raise NexorError(f"HTTP {status_code}: {body}", status_code, body)
