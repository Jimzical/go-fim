"""View-layer helpers for the dashboard routes.

Pure shaping logic: SQLite Row → template-friendly dict, plus relative-time
and freshness-dot mapping. No FastAPI / no DB calls — keeps app.py focused on
routes."""

import json
from datetime import datetime, timezone
from sqlite3 import Row

# Stored kind strings → display symbol + CSS class. Mirrors store.ChangeKind.Symbol().
KIND_SYMBOLS = {
    "created": ("+", "c"),
    "modified": ("~", "m"),
    "deleted": ("−", "d"),
}


def parse_ts(s: str | None) -> datetime | None:
    if not s:
        return None
    return datetime.fromisoformat(s)


def relative(dt: datetime | None) -> str:
    if dt is None:
        return "never"
    secs = int((datetime.now(timezone.utc) - dt).total_seconds())
    if secs < 0:
        return "just now"
    if secs < 60:
        return f"{secs}s ago"
    if secs < 3600:
        return f"{secs // 60}m ago"
    if secs < 86_400:
        return f"{secs // 3600}h ago"
    return f"{secs // 86_400}d ago"


def freshness(dt: datetime | None) -> str:
    if dt is None:
        return "grey"
    secs = (datetime.now(timezone.utc) - dt).total_seconds()
    if secs < 3600:
        return "green"
    if secs < 86_400:
        return "amber"
    return "red"


def agent_view(row: Row) -> dict:
    """Shape an `agents` row for the index/detail templates."""
    last = parse_ts(row["last_report_at"])
    first = parse_ts(row["first_seen"])
    return {
        "id": row["id"],
        "name": row["name"],
        "scan_path": row["scan_path"],
        "first_seen": row["first_seen"],
        "first_seen_rel": relative(first),
        "last_report_at": row["last_report_at"],
        "last_report_rel": relative(last),
        "freshness": freshness(last),
    }


CHANGES_DISPLAY_CAP = 200


def report_view(row: Row) -> dict:
    """Shape a `reports` row (raw JSON in `json` col) for the timeline template."""
    payload = json.loads(row["json"])
    all_changes = []
    for c in payload.get("changes", []):
        sym, cls = KIND_SYMBOLS.get(c["kind"], ("?", ""))
        all_changes.append({"symbol": sym, "sym_class": cls, "path": c["path"]})
    truncated = max(0, len(all_changes) - CHANGES_DISPLAY_CAP)
    return {
        "ts": row["ts"],
        "ts_rel": relative(parse_ts(row["ts"])),
        "total_files": payload.get("total_files", 0),
        "num_created": payload.get("num_created", 0),
        "num_modified": payload.get("num_modified", 0),
        "num_deleted": payload.get("num_deleted", 0),
        "changes": all_changes[:CHANGES_DISPLAY_CAP],
        "changes_truncated": truncated,
    }
