"""All SQLite queries in one place. Each function takes a Connection as its
first argument so they can be tested against an in-memory DB without touching
the global conn from db.py."""

import secrets
from sqlite3 import Connection, Row


def find_agent_by_id(conn: Connection, agent_id: str) -> Row | None:
    return conn.execute(
        "SELECT id, name, scan_path, first_seen, last_report_at, api_token "
        "FROM agents WHERE id = ?",
        (agent_id,),
    ).fetchone()


def list_agents(conn: Connection) -> list[Row]:
    return conn.execute(
        "SELECT id, name, scan_path, first_seen, last_report_at FROM agents "
        "ORDER BY last_report_at IS NULL, last_report_at DESC"
    ).fetchall()


def refresh_agent(
    conn: Connection,
    agent_id: str,
    name: str,
    scan_path: str,
    now: str,
) -> None:
    """Update the display fields + last_report_at on an existing agent row.
    The row is created at /api/setup time (or pre-seeded for demos), so /report
    only ever updates here — there is no insert path. Operator-controlled
    fields (name, scan_path) follow the most recent /report so YAML edits
    propagate without manual cleanup."""
    conn.execute(
        "UPDATE agents SET name = ?, scan_path = ?, last_report_at = ? WHERE id = ?",
        (name, scan_path, now, agent_id),
    )


def register_agent(
    conn: Connection,
    agent_id: str,
    name: str,
    scan_path: str,
    now: str,
) -> str | None:
    """Insert the agent row and generate an API token. Returns the token if
    a new row was created, None if the agent_id was already registered."""
    token = secrets.token_urlsafe(32)
    cur = conn.execute(
        """
        INSERT OR IGNORE INTO agents (id, name, scan_path, first_seen, last_report_at, api_token)
        VALUES (?, ?, ?, ?, NULL, ?)
        """,
        (agent_id, name, scan_path, now, token),
    )
    return token if cur.rowcount > 0 else None


def save_report(
    conn: Connection,
    agent_id: str,
    ts: str,
    payload: str,
    retention_n: int,
) -> None:
    """Non-transactional version - caller manages transaction."""
    conn.execute(
        "INSERT INTO reports (agent_id, ts, json) VALUES (?, ?, ?)",
        (agent_id, ts, payload),
    )
    conn.execute(
        """
        DELETE FROM reports
        WHERE agent_id = ?
          AND id NOT IN (
            SELECT id FROM reports
            WHERE agent_id = ?
            ORDER BY ts DESC
            LIMIT ?
          )
        """,
        (agent_id, agent_id, retention_n),
    )


def list_agent_reports(conn: Connection, agent_id: str, limit: int) -> list[Row]:
    return conn.execute(
        "SELECT ts, json FROM reports WHERE agent_id = ? "
        "ORDER BY ts DESC LIMIT ?",
        (agent_id, limit),
    ).fetchall()
