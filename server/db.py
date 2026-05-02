"""SQLite connection + schema. Routes are async def so they run on the
single event-loop thread — no concurrent access, no threading needed."""

import os
import sqlite3
from pathlib import Path

DB_PATH = Path(os.environ.get("GOFIM_DB_PATH") or (Path(__file__).parent / "server.db"))

conn = sqlite3.connect(DB_PATH, check_same_thread=False)
conn.row_factory = sqlite3.Row
conn.execute("PRAGMA foreign_keys = ON")


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
          last_report_at  TIMESTAMP
        );
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
    conn.commit()
