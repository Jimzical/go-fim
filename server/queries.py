"""All SQLite queries in one place. Each function takes a Connection as its
first argument so they can be tested against an in-memory DB without touching
the global conn from db.py."""

from sqlite3 import Connection, Row


def find_agent_by_id(conn: Connection, agent_id: str) -> Row | None:
    return conn.execute(
        "SELECT id, name, scan_path, first_seen, last_report_at "
        "FROM agents WHERE id = ?",
        (agent_id,),
    ).fetchone()


def list_agents(conn: Connection) -> list[Row]:
    return conn.execute(
        "SELECT id, name, scan_path, first_seen, last_report_at FROM agents "
        "ORDER BY last_report_at IS NULL, last_report_at DESC"
    ).fetchall()


def upsert_agent(
    conn: Connection,
    agent_id: str,
    name: str,
    scan_path: str,
    now: str,
) -> None:
    """Non-transactional version - caller manages transaction.
    Create the agent row on first sight, otherwise refresh name + scan_path
    + last_report_at. Operator-controlled fields (name, scan_path) follow the
    most recent /report — YAML edits propagate without manual cleanup."""
    conn.execute(
        """
        INSERT INTO agents (id, name, scan_path, first_seen, last_report_at)
        VALUES (?, ?, ?, ?, ?)
        ON CONFLICT (id) DO UPDATE SET
          name = excluded.name,
          scan_path = excluded.scan_path,
          last_report_at = excluded.last_report_at
        """,
        (agent_id, name, scan_path, now, now),
    )


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
