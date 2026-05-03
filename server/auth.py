"""JWT minting / verification for the agent-setup handshake.

The signing secret is shared across server restarts: persisted to a file under
the same directory as the SQLite db so the existing server-data volume keeps
it alive. Override via GOFIM_JWT_SECRET in production.
"""

import os
import secrets
import time

import jwt

from .db import DB_PATH

ISSUER = "go-fim-server"
TTL_SECONDS = 3600  # 1 hour

_secret: str | None = None


def load_or_init_secret() -> str:
    """Idempotent: env var wins, else read/create a file beside the db."""
    global _secret
    if _secret is not None:
        return _secret

    env = os.environ.get("GOFIM_JWT_SECRET")
    if env:
        _secret = env
        return _secret

    secret_path = DB_PATH.parent / "jwt_secret"
    if secret_path.exists():
        _secret = secret_path.read_text().strip()
        return _secret

    new_secret = secrets.token_urlsafe(32)
    secret_path.write_text(new_secret)
    secret_path.chmod(0o600)
    _secret = new_secret
    return _secret


def mint_setup_token(agent_name: str, scan_path: str, excludes: list[str]) -> str:
    now = int(time.time())
    claims = {
        "iss": ISSUER,
        "iat": now,
        "exp": now + TTL_SECONDS,
        "agent_name": agent_name,
        "scan_path": scan_path,
        "excludes": excludes,
    }
    return jwt.encode(claims, load_or_init_secret(), algorithm="HS256")


def verify_setup_token(token: str) -> dict:
    """Returns the validated claims. Raises jwt.InvalidTokenError on failure."""
    return jwt.decode(
        token,
        load_or_init_secret(),
        algorithms=["HS256"],
        issuer=ISSUER,
        options={"require": ["exp", "iat", "agent_name", "scan_path"]},
    )
