"""In-memory token bucket rate limiter and middleware for FastAPI.

Provides token bucket rate limiting on targeted endpoints (/report and /api/setup)
with standard HTTP 429 Too Many Requests responses and Retry-After headers.
Key expiration ensures memory usage stays low without external dependencies.
"""

import math
import os
import threading
import time
from typing import Callable, Dict, Optional, Set, Tuple
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.requests import Request
from starlette.responses import JSONResponse, Response


class TokenBucket:
    """In-memory token bucket algorithm implementation."""

    def __init__(self, capacity: float, refill_rate: float, now: Optional[float] = None) -> None:
        """
        capacity: Maximum number of tokens the bucket can hold.
        refill_rate: Number of tokens added per second.
        """
        self.capacity = float(capacity)
        self.refill_rate = float(refill_rate)
        self.tokens = float(capacity)
        t = time.monotonic() if now is None else now
        self.last_refill = t
        self.last_access = t

    def consume(self, count: float = 1.0, now: Optional[float] = None) -> Tuple[bool, int]:
        """
        Attempts to consume `count` tokens.
        Returns (allowed: bool, retry_after_seconds: int).
        If allowed is True, retry_after_seconds is 0.
        If allowed is False, retry_after_seconds is the number of seconds until
        at least `count` tokens are available (minimum 1 second).
        """
        current_time = time.monotonic() if now is None else now
        elapsed = current_time - self.last_refill
        if elapsed > 0:
            self.tokens = min(self.capacity, self.tokens + elapsed * self.refill_rate)
            self.last_refill = current_time
        self.last_access = current_time

        if self.tokens >= count:
            self.tokens -= count
            return True, 0

        # Calculate time needed to accumulate required tokens
        needed = count - self.tokens
        if self.refill_rate > 0:
            retry_after = math.ceil(needed / self.refill_rate)
        else:
            retry_after = 60
        return False, max(1, int(retry_after))


class TokenBucketRateLimiter:
    """Manages token buckets for client IPs and paths with key expiration."""

    def __init__(
        self,
        rate_limits: Optional[Dict[str, Tuple[float, float]]] = None,
        ttl_seconds: float = 300.0,
        cleanup_interval_seconds: float = 60.0,
    ) -> None:
        """
        rate_limits: mapping of path -> (capacity, refill_rate_per_sec)
        ttl_seconds: time after which idle, full buckets are expired
        cleanup_interval_seconds: minimum interval between cleanup sweeps
        """
        # Default limits for sensitive endpoints
        # Can be configured via environment variables
        report_cap = float(os.environ.get("GOFIM_RATE_LIMIT_REPORT_CAPACITY", "20.0"))
        report_rate = float(os.environ.get("GOFIM_RATE_LIMIT_REPORT_RATE", "5.0"))
        setup_cap = float(os.environ.get("GOFIM_RATE_LIMIT_SETUP_CAPACITY", "5.0"))
        setup_rate = float(os.environ.get("GOFIM_RATE_LIMIT_SETUP_RATE", "1.0"))

        self.rate_limits: Dict[str, Tuple[float, float]] = rate_limits or {
            "/report": (report_cap, report_rate),
            "/api/setup": (setup_cap, setup_rate),
        }
        self.ttl_seconds = ttl_seconds
        self.cleanup_interval_seconds = cleanup_interval_seconds
        self.last_cleanup: Optional[float] = None
        self._buckets: Dict[Tuple[str, str], TokenBucket] = {}
        self._lock = threading.Lock()

    def get_client_ip(self, request: Request) -> str:
        """Extracts client IP, respecting X-Forwarded-For if present."""
        forwarded = request.headers.get("X-Forwarded-For")
        if forwarded:
            # First IP in comma-separated list is the original client
            return forwarded.split(",")[0].strip()
        if request.client and request.client.host:
            return request.client.host
        return "127.0.0.1"

    def check_rate_limit(
        self, client_ip: str, path: str, now: Optional[float] = None
    ) -> Tuple[bool, int]:
        """
        Checks rate limit for client_ip on path.
        Returns (allowed: bool, retry_after_seconds: int).
        """
        norm_path = path.rstrip("/")
        if norm_path not in self.rate_limits:
            return True, 0

        current_time = time.monotonic() if now is None else now
        capacity, rate = self.rate_limits[norm_path]
        key = (client_ip, norm_path)

        with self._lock:
            self._maybe_cleanup(current_time)
            bucket = self._buckets.get(key)
            if bucket is None:
                bucket = TokenBucket(capacity, rate, now=current_time)
                self._buckets[key] = bucket

            return bucket.consume(1.0, now=current_time)

    def _maybe_cleanup(self, now: float) -> None:
        """Evicts expired buckets whose idle time exceeds ttl_seconds and have refilled."""
        if self.last_cleanup is None:
            self.last_cleanup = now
            return

        if now - self.last_cleanup < self.cleanup_interval_seconds:
            return

        self.last_cleanup = now
        expired_keys = []
        for key, bucket in self._buckets.items():
            # If idle past TTL and tokens have refilled close to capacity, remove bucket
            elapsed = now - bucket.last_refill
            refilled = min(bucket.capacity, bucket.tokens + elapsed * bucket.refill_rate)
            if (now - bucket.last_access > self.ttl_seconds) and (refilled >= bucket.capacity):
                expired_keys.append(key)

        for key in expired_keys:
            del self._buckets[key]

    def reset(self) -> None:
        """Clears all buckets (useful in testing)."""
        with self._lock:
            self._buckets.clear()
            self.last_cleanup = time.monotonic()


class TokenBucketMiddleware(BaseHTTPMiddleware):
    """FastAPI/Starlette middleware enforcing token bucket rate limits."""

    def __init__(
        self,
        app,
        limiter: Optional[TokenBucketRateLimiter] = None,
        enabled: Optional[bool] = None,
    ) -> None:
        super().__init__(app)
        self.limiter = limiter or TokenBucketRateLimiter()
        if enabled is not None:
            self.enabled = enabled
        else:
            self.enabled = os.environ.get("GOFIM_RATE_LIMIT_ENABLED", "true").lower() in (
                "true",
                "1",
                "yes",
            )

    async def dispatch(self, request: Request, call_next: Callable) -> Response:
        if not self.enabled:
            return await call_next(request)

        norm_path = request.url.path.rstrip("/")
        if norm_path in self.limiter.rate_limits:
            client_ip = self.limiter.get_client_ip(request)
            allowed, retry_after = self.limiter.check_rate_limit(client_ip, norm_path)
            if not allowed:
                return JSONResponse(
                    status_code=429,
                    content={
                        "detail": f"Rate limit exceeded. Try again in {retry_after} seconds."
                    },
                    headers={"Retry-After": str(retry_after)},
                )

        return await call_next(request)
