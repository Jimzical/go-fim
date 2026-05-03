"""SQLite connection + schema. Routes are async def so they run on the
single event-loop thread — no concurrent access, no threading needed."""

import os
import sqlite3
from datetime import datetime, timezone
from pathlib import Path

DB_PATH = Path(os.environ.get("GOFIM_DB_PATH") or (Path(__file__).parent / "server.db"))

conn = sqlite3.connect(DB_PATH, check_same_thread=False)
conn.row_factory = sqlite3.Row
conn.execute("PRAGMA foreign_keys = ON")

# Demo fixtures: when GOFIM_DEMO_MODE=1, init_db pre-seeds these UUIDs so the
# demo containers can post /report without going through --setup. Their
# YAML configs (demo/*.gofim.yml) pin the same UUIDs via the agent_id field.
# scan_path here is a placeholder — first /report refreshes it to whatever the
# agent actually scanned.
DEMO_AGENTS = [
    ("11111111-1111-1111-1111-111111111111", "agent-alpha",   "demo:/data/alpha"),
    ("22222222-2222-2222-2222-222222222222", "agent-bravo",   "demo:/data/bravo"),
    ("33333333-3333-3333-3333-333333333333", "agent-charlie", "demo:/data/charlie"),
]


def init_db() -> None:
    # Name is intentionally non-unique: identity is the agent_id (UUID); name
    # and scan_path are operator-supplied display fields and update on every
    # /report. If two agents happen to share a name, dashboard shows both
    # rows and they're distinguished by id.
    conn.executescript(
        """
        CREATE TABLE IF NOT EXISTS agents (
          id              TEXT PRIMARY KEY,
          name            TEXT NOT NULL,
          scan_path       TEXT NOT NULL DEFAULT '',
          first_seen      TIMESTAMP NOT NULL,
          last_report_at  TIMESTAMP,
          api_token       TEXT
        );
        CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_api_token
          ON agents (api_token) WHERE api_token IS NOT NULL;
        CREATE TABLE IF NOT EXISTS reports (
          id         INTEGER PRIMARY KEY AUTOINCREMENT,
          agent_id   TEXT NOT NULL REFERENCES agents(id),
          ts         TIMESTAMP NOT NULL,
          json       TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS reports_agent_ts
          ON reports (agent_id, ts DESC);
        """
    )
    if os.environ.get("GOFIM_DEMO_MODE") == "1":
        now = datetime.now(timezone.utc).isoformat()
        with conn:
            conn.executemany(
                "INSERT OR IGNORE INTO agents (id, name, scan_path, first_seen, last_report_at) "
                "VALUES (?, ?, ?, ?, NULL)",
                [(uid, name, path, now) for uid, name, path in DEMO_AGENTS],
            )
    conn.commit()
