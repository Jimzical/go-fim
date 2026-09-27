from datetime import datetime, timezone
import pytest
from fastapi.testclient import TestClient

from server import auth, db
from server.app import app, registered_agents


@pytest.fixture
def client(tmp_path, monkeypatch):
    db_file = tmp_path / "test_server.db"
    monkeypatch.setenv("GOFIM_DB_PATH", str(db_file))
    monkeypatch.setenv("GOFIM_JWT_SECRET", "test-secret-key-32-chars-long-12345")

    # Re-initialize DB and secret
    db.DB_PATH = db_file
    db.conn = db.sqlite3.connect(db_file, check_same_thread=False)
    db.conn.row_factory = db.sqlite3.Row
    db.conn.execute("PRAGMA foreign_keys = ON")

    # Patch server module conn references
    monkeypatch.setattr("server.app.conn", db.conn)

    auth._secret = None
    auth.load_or_init_secret()

    db.init_db()
    registered_agents.clear()
    registered_agents.update(r["id"] for r in db.conn.execute("SELECT id FROM agents"))

    with TestClient(app) as tc:
        yield tc

    db.conn.close()


def test_agent_setup_and_report_handshake(client):
    # 1. Form submit POST /agents/new
    res = client.post(
        "/agents/new",
        data={
            "agent_name": "test-agent-1",
            "scan_path": "/var/log",
            "excludes": "*.tmp\n*.log",
        },
    )
    assert res.status_code == 200

    setup_token = auth.mint_setup_token("test-agent-1", "/var/log", ["*.tmp", "*.log"])

    # 2. Call /api/setup with setup token
    agent_id = "aaaaaaaa-1111-2222-3333-bbbbbbbbbbbb"
    setup_res = client.post(
        "/api/setup",
        json={"agent_id": agent_id},
        headers={"Authorization": f"Bearer {setup_token}"},
    )
    assert setup_res.status_code == 200
    data = setup_res.json()
    assert data["agent_id"] == agent_id
    assert data["agent_name"] == "test-agent-1"
    assert data["scan_path"] == "/var/log"
    agent_token = data["api_token"]
    assert agent_token is not None

    # Verify returned token is a valid Agent JWT statelessly
    claims = auth.verify_agent_token(agent_token)
    assert claims["sub"] == agent_id

    # 3. Post /report with valid Agent JWT
    report_payload = {
        "agent_id": agent_id,
        "agent_name": "test-agent-1",
        "scan_path": "/var/log",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total_files": 10,
        "num_created": 1,
        "num_modified": 0,
        "num_deleted": 0,
        "changes": [{"kind": "created", "path": "/var/log/test.txt"}],
    }
    rep_res = client.post(
        "/report",
        json=report_payload,
        headers={"Authorization": f"Bearer {agent_token}"},
    )
    assert rep_res.status_code == 200


def test_setup_duplicate_agent(client):
    setup_token = auth.mint_setup_token("test-agent", "/path", [])
    agent_id = "bbbbbbbb-1111-2222-3333-bbbbbbbbbbbb"

    # First setup: success
    res1 = client.post(
        "/api/setup",
        json={"agent_id": agent_id},
        headers={"Authorization": f"Bearer {setup_token}"},
    )
    assert res1.status_code == 200

    # Second setup: conflict
    res2 = client.post(
        "/api/setup",
        json={"agent_id": agent_id},
        headers={"Authorization": f"Bearer {setup_token}"},
    )
    assert res2.status_code == 409


def test_setup_unauthorized(client):
    agent_id = "cccccccc-1111-2222-3333-bbbbbbbbbbbb"
    # Missing token
    res1 = client.post("/api/setup", json={"agent_id": agent_id})
    assert res1.status_code == 401

    # Invalid token
    res2 = client.post(
        "/api/setup",
        json={"agent_id": agent_id},
        headers={"Authorization": "Bearer invalid.jwt.token"},
    )
    assert res2.status_code == 401


def test_report_unauthorized_missing_token(client):
    agent_id = "dddddddd-1111-2222-3333-bbbbbbbbbbbb"
    registered_agents.add(agent_id)
    report_payload = {
        "agent_id": agent_id,
        "agent_name": "test",
        "scan_path": "/path",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total_files": 1,
        "num_created": 0,
        "num_modified": 0,
        "num_deleted": 0,
        "changes": [],
    }
    res = client.post("/report", json=report_payload)
    assert res.status_code == 401


def test_report_unauthorized_invalid_token(client):
    agent_id = "eeeeeeee-1111-2222-3333-bbbbbbbbbbbb"
    registered_agents.add(agent_id)
    report_payload = {
        "agent_id": agent_id,
        "agent_name": "test",
        "scan_path": "/path",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total_files": 1,
        "num_created": 0,
        "num_modified": 0,
        "num_deleted": 0,
        "changes": [],
    }
    res = client.post(
        "/report",
        json=report_payload,
        headers={"Authorization": "Bearer invalid-token"},
    )
    assert res.status_code == 401


def test_report_unauthorized_mismatched_agent_id(client):
    agent1_id = "11111111-0000-0000-0000-000000000001"
    agent2_id = "22222222-0000-0000-0000-000000000002"
    registered_agents.add(agent1_id)
    registered_agents.add(agent2_id)

    # Token minted for agent1_id
    token_agent1 = auth.mint_agent_token(agent1_id)

    # Report sent for agent2_id using token_agent1
    report_payload = {
        "agent_id": agent2_id,
        "agent_name": "test-agent-2",
        "scan_path": "/path",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total_files": 1,
        "num_created": 0,
        "num_modified": 0,
        "num_deleted": 0,
        "changes": [],
    }
    res = client.post(
        "/report",
        json=report_payload,
        headers={"Authorization": f"Bearer {token_agent1}"},
    )
    assert res.status_code == 401


def test_report_unregistered_agent(client):
    unregistered_id = "ffffffff-1111-2222-3333-bbbbbbbbbbbb"
    token = auth.mint_agent_token(unregistered_id)
    report_payload = {
        "agent_id": unregistered_id,
        "agent_name": "unregistered",
        "scan_path": "/path",
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "total_files": 1,
        "num_created": 0,
        "num_modified": 0,
        "num_deleted": 0,
        "changes": [],
    }
    res = client.post(
        "/report",
        json=report_payload,
        headers={"Authorization": f"Bearer {token}"},
    )
    assert res.status_code == 403
