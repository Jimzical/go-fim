import time
import jwt
import pytest

from server import auth


@pytest.fixture(autouse=True)
def setup_jwt_secret(tmp_path, monkeypatch):
    monkeypatch.setenv("GOFIM_JWT_SECRET", "test-secret-key-32-chars-long-12345")
    auth._secret = None
    auth.load_or_init_secret()


def test_setup_token_roundtrip():
    token = auth.mint_setup_token("agent-1", "/var/log", ["*.tmp"])
    claims = auth.verify_setup_token(token)
    assert claims["agent_name"] == "agent-1"
    assert claims["scan_path"] == "/var/log"
    assert claims["excludes"] == ["*.tmp"]
    assert claims["iss"] == auth.ISSUER


def test_agent_token_roundtrip():
    agent_id = "11111111-2222-3333-4444-555555555555"
    token = auth.mint_agent_token(agent_id)
    claims = auth.verify_agent_token(token)
    assert claims["sub"] == agent_id
    assert claims["iss"] == auth.ISSUER
    assert "iat" in claims


def test_verify_agent_token_invalid_signature():
    agent_id = "11111111-2222-3333-4444-555555555555"
    bad_token = jwt.encode({"iss": auth.ISSUER, "sub": agent_id, "iat": int(time.time())}, "wrong-secret", algorithm="HS256")
    with pytest.raises(jwt.InvalidTokenError):
        auth.verify_agent_token(bad_token)


def test_verify_agent_token_wrong_issuer():
    agent_id = "11111111-2222-3333-4444-555555555555"
    bad_token = jwt.encode({"iss": "wrong-issuer", "sub": agent_id, "iat": int(time.time())}, auth.load_or_init_secret(), algorithm="HS256")
    with pytest.raises(jwt.InvalidTokenError):
        auth.verify_agent_token(bad_token)


def test_verify_agent_token_missing_sub():
    bad_token = jwt.encode({"iss": auth.ISSUER, "iat": int(time.time())}, auth.load_or_init_secret(), algorithm="HS256")
    with pytest.raises(jwt.InvalidTokenError):
        auth.verify_agent_token(bad_token)


def test_cross_token_rejection():
    setup_token = auth.mint_setup_token("agent-1", "/var/log", [])
    agent_token = auth.mint_agent_token("11111111-2222-3333-4444-555555555555")

    # Setup token has no sub, should be rejected by verify_agent_token
    with pytest.raises(jwt.InvalidTokenError):
        auth.verify_agent_token(setup_token)

    # Agent token has no exp/agent_name/scan_path, should be rejected by verify_setup_token
    with pytest.raises(jwt.InvalidTokenError):
        auth.verify_setup_token(agent_token)
