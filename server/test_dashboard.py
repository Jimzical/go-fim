import unittest
from datetime import datetime, timezone, timedelta
from sqlite3 import Row
import sqlite3

from server.dashboard import parse_ts, relative, freshness, agent_view, report_view


class TestDashboardHelpers(unittest.TestCase):
    def test_parse_ts_none_and_empty(self):
        self.assertIsNone(parse_ts(None))
        self.assertIsNone(parse_ts(""))

    def test_parse_ts_naive_iso(self):
        ts_str = "2026-09-27T06:30:00"
        dt = parse_ts(ts_str)
        self.assertIsNotNone(dt)
        self.assertEqual(dt.tzinfo, timezone.utc)
        self.assertEqual(dt, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

    def test_parse_ts_naive_iso_with_space(self):
        ts_str = "2026-09-27 06:30:00"
        dt = parse_ts(ts_str)
        self.assertIsNotNone(dt)
        self.assertEqual(dt.tzinfo, timezone.utc)
        self.assertEqual(dt, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

    def test_parse_ts_aware_utc_z(self):
        ts_str = "2026-09-27T06:30:00Z"
        dt = parse_ts(ts_str)
        self.assertIsNotNone(dt)
        self.assertEqual(dt.tzinfo, timezone.utc)
        self.assertEqual(dt, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

    def test_parse_ts_aware_utc_offset(self):
        ts_str = "2026-09-27T06:30:00+00:00"
        dt = parse_ts(ts_str)
        self.assertIsNotNone(dt)
        self.assertEqual(dt.tzinfo, timezone.utc)
        self.assertEqual(dt, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

    def test_parse_ts_aware_non_utc_offset(self):
        # 08:30 in +02:00 is 06:30 UTC
        ts_str = "2026-09-27T08:30:00+02:00"
        dt = parse_ts(ts_str)
        self.assertIsNotNone(dt)
        self.assertEqual(dt.tzinfo, timezone.utc)
        self.assertEqual(dt, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

        # 01:30 in -05:00 is 06:30 UTC
        ts_str_neg = "2026-09-27T01:30:00-05:00"
        dt_neg = parse_ts(ts_str_neg)
        self.assertIsNotNone(dt_neg)
        self.assertEqual(dt_neg.tzinfo, timezone.utc)
        self.assertEqual(dt_neg, datetime(2026, 9, 27, 6, 30, 0, tzinfo=timezone.utc))

    def test_relative_none(self):
        self.assertEqual(relative(None), "never")

    def test_relative_naive_datetime(self):
        now_utc = datetime.now(timezone.utc)

        # Naive datetime 30s ago
        naive_recent = (now_utc - timedelta(seconds=30)).replace(tzinfo=None)
        self.assertEqual(relative(naive_recent), "30s ago")

        # Naive datetime 5 minutes ago
        naive_5m = (now_utc - timedelta(minutes=5)).replace(tzinfo=None)
        self.assertEqual(relative(naive_5m), "5m ago")

        # Naive datetime 2 hours ago
        naive_2h = (now_utc - timedelta(hours=2)).replace(tzinfo=None)
        self.assertEqual(relative(naive_2h), "2h ago")

        # Naive datetime 3 days ago
        naive_3d = (now_utc - timedelta(days=3)).replace(tzinfo=None)
        self.assertEqual(relative(naive_3d), "3d ago")

    def test_relative_future_datetime(self):
        now_utc = datetime.now(timezone.utc)
        future_naive = (now_utc + timedelta(seconds=10)).replace(tzinfo=None)
        self.assertEqual(relative(future_naive), "just now")

    def test_freshness_none(self):
        self.assertEqual(freshness(None), "grey")

    def test_freshness_naive_datetime(self):
        now_utc = datetime.now(timezone.utc)

        # < 1 hour -> green
        dt_green = (now_utc - timedelta(minutes=30)).replace(tzinfo=None)
        self.assertEqual(freshness(dt_green), "green")

        # 1-24 hours -> amber
        dt_amber = (now_utc - timedelta(hours=5)).replace(tzinfo=None)
        self.assertEqual(freshness(dt_amber), "amber")

        # > 24 hours -> red
        dt_red = (now_utc - timedelta(hours=25)).replace(tzinfo=None)
        self.assertEqual(freshness(dt_red), "red")

    def test_agent_view_with_naive_timestamps(self):
        db = sqlite3.connect(":memory:")
        db.row_factory = sqlite3.Row
        cur = db.cursor()
        cur.execute(
            "CREATE TABLE agents (id TEXT, name TEXT, scan_path TEXT, first_seen TEXT, last_report_at TEXT)"
        )
        cur.execute(
            "INSERT INTO agents VALUES (?, ?, ?, ?, ?)",
            ("agent-123", "test-agent", "/var/log", "2026-09-27T01:00:00", "2026-09-27T06:00:00")
        )
        row = cur.execute("SELECT * FROM agents").fetchone()

        view = agent_view(row)
        self.assertEqual(view["id"], "agent-123")
        self.assertIsNotNone(view["first_seen_rel"])
        self.assertIsNotNone(view["last_report_rel"])
        self.assertIn(view["freshness"], ("green", "amber", "red"))

    def test_report_view_with_naive_timestamps(self):
        db = sqlite3.connect(":memory:")
        db.row_factory = sqlite3.Row
        cur = db.cursor()
        cur.execute("CREATE TABLE reports (ts TEXT, json TEXT)")
        cur.execute(
            "INSERT INTO reports VALUES (?, ?)",
            (
                "2026-09-27T06:00:00",
                '{"total_files": 10, "num_created": 1, "num_modified": 0, "num_deleted": 0, "changes": [{"kind": "created", "path": "/a/b"}]}'
            )
        )
        row = cur.execute("SELECT * FROM reports").fetchone()

        view = report_view(row)
        self.assertEqual(view["ts"], "2026-09-27T06:00:00")
        self.assertIsNotNone(view["ts_rel"])


if __name__ == "__main__":
    unittest.main()
