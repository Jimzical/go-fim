"""Unit tests for token bucket rate limiting on FastAPI endpoints."""

import os
import time
import unittest
from starlette.testclient import TestClient

# Ensure temporary test DB path is used before importing server
os.environ["GOFIM_DB_PATH"] = "/tmp/test_server.db"

from server.app import app, rate_limiter
from server.ratelimit import TokenBucket, TokenBucketRateLimiter


class TestTokenBucket(unittest.TestCase):
    def test_consume_within_capacity(self):
        bucket = TokenBucket(capacity=3, refill_rate=1.0, now=100.0)
        allowed, retry_after = bucket.consume(1.0, now=100.0)
        self.assertTrue(allowed)
        self.assertEqual(retry_after, 0)

        allowed, retry_after = bucket.consume(2.0, now=100.0)
        self.assertTrue(allowed)
        self.assertEqual(retry_after, 0)

    def test_consume_exceeding_capacity(self):
        bucket = TokenBucket(capacity=2, refill_rate=1.0, now=100.0)
        self.assertTrue(bucket.consume(1.0, now=100.0)[0])
        self.assertTrue(bucket.consume(1.0, now=100.0)[0])

        # Exceeds capacity
        allowed, retry_after = bucket.consume(1.0, now=100.0)
        self.assertFalse(allowed)
        self.assertGreaterEqual(retry_after, 1)

    def test_refill_over_time(self):
        bucket = TokenBucket(capacity=2, refill_rate=1.0, now=100.0)
        self.assertTrue(bucket.consume(2.0, now=100.0)[0])
        # Immediately empty
        self.assertFalse(bucket.consume(1.0, now=100.0)[0])

        # After 1.5 seconds, 1.5 tokens refilled -> can consume 1
        allowed, retry_after = bucket.consume(1.0, now=101.5)
        self.assertTrue(allowed)
        self.assertEqual(retry_after, 0)


class TestRateLimiterCleanup(unittest.TestCase):
    def test_key_expiration(self):
        limiter = TokenBucketRateLimiter(
            rate_limits={"/test": (2.0, 1.0)},
            ttl_seconds=10.0,
            cleanup_interval_seconds=5.0,
        )
        # Access at t=0
        limiter.check_rate_limit("1.2.3.4", "/test", now=100.0)
        self.assertIn(("1.2.3.4", "/test"), limiter._buckets)

        # Before TTL at t=104
        limiter.check_rate_limit("5.6.7.8", "/test", now=104.0)
        self.assertIn(("1.2.3.4", "/test"), limiter._buckets)
        self.assertIn(("5.6.7.8", "/test"), limiter._buckets)

        # After TTL (t=112 > 100 + 10) and after cleanup interval (112 - 100 > 5)
        limiter.check_rate_limit("5.6.7.8", "/test", now=112.0)
        self.assertNotIn(("1.2.3.4", "/test"), limiter._buckets)
        self.assertIn(("5.6.7.8", "/test"), limiter._buckets)


class TestFastAPIRateLimiting(unittest.TestCase):
    def setUp(self):
        rate_limiter.reset()
        # Set low limits for testing
        rate_limiter.rate_limits = {
            "/report": (2.0, 0.5),
            "/api/setup": (2.0, 0.5),
        }

    def tearDown(self):
        rate_limiter.reset()

    def test_report_endpoint_rate_limit(self):
        with TestClient(app) as client:
            headers = {"X-Forwarded-For": "192.168.1.100"}
            payload = {
                "agent_id": "00000000-0000-0000-0000-000000000001",
                "agent_name": "test-agent",
                "scan_path": "/test",
                "timestamp": "2026-11-04T00:00:00Z",
                "total_files": 10,
                "summary": {
                    "num_created": 0,
                    "num_modified": 0,
                    "num_deleted": 0,
                    "changes": [],
                },
            }

            # 1st request: accepted (or 403 because agent not registered, but middleware allows it through)
            resp1 = client.post("/report", json=payload, headers=headers)
            self.assertNotEqual(resp1.status_code, 429)

            # 2nd request: allowed through middleware
            resp2 = client.post("/report", json=payload, headers=headers)
            self.assertNotEqual(resp2.status_code, 429)

            # 3rd request: rate limit exceeded -> 429
            resp3 = client.post("/report", json=payload, headers=headers)
            self.assertEqual(resp3.status_code, 429)
            self.assertIn("Retry-After", resp3.headers)
            retry_after = int(resp3.headers["Retry-After"])
            self.assertGreaterEqual(retry_after, 1)
            self.assertIn("detail", resp3.json())

    def test_setup_endpoint_rate_limit(self):
        with TestClient(app) as client:
            headers = {"X-Forwarded-For": "192.168.1.200"}
            payload = {"agent_id": "00000000-0000-0000-0000-000000000002"}

            # First 2 requests pass through middleware
            resp1 = client.post("/api/setup", json=payload, headers=headers)
            self.assertNotEqual(resp1.status_code, 429)

            resp2 = client.post("/api/setup", json=payload, headers=headers)
            self.assertNotEqual(resp2.status_code, 429)

            # 3rd request hits rate limit
            resp3 = client.post("/api/setup", json=payload, headers=headers)
            self.assertEqual(resp3.status_code, 429)
            self.assertIn("Retry-After", resp3.headers)
            retry_after = int(resp3.headers["Retry-After"])
            self.assertGreaterEqual(retry_after, 1)


if __name__ == "__main__":
    unittest.main()
