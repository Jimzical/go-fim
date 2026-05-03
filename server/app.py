"""FastAPI control plane for go-fim agents.

Identity is the agent_id (UUID); agent_name and scan_path are display-only
fields refreshed on every /report. /report is gated by registered_agents:
agents must complete /api/setup (or be pre-seeded as a demo) first.
"""

import os
from contextlib import asynccontextmanager
from datetime import datetime, timezone
from pathlib import Path

import jwt
from fastapi import Depends, FastAPI, Form, HTTPException, Request
from fastapi.middleware.gzip import GZipMiddleware
from fastapi.responses import HTMLResponse
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer
from fastapi.templating import Jinja2Templates

from . import auth, queries
from .dashboard import agent_view, report_view
from .db import conn, init_db
from .models import ReportPayload, ReportResp, SetupReq, SetupResp

RETENTION_N = 50
DASHBOARD_N = 10

templates = Jinja2Templates(directory=Path(__file__).parent / "templates")

bearer = HTTPBearer(auto_error=False)

# In-memory mirror of `SELECT id FROM agents`.
registered_agents: set[str] = set()


@asynccontextmanager
async def lifespan(app: FastAPI):
    init_db()
    auth.load_or_init_secret()  # warm up; fail fast if the secret can't be persisted
    registered_agents.update(r["id"] for r in conn.execute("SELECT id FROM agents"))
    yield


app = FastAPI(lifespan=lifespan, title="go-fim control plane", docs_url=None, redoc_url=None)
app.add_middleware(GZipMiddleware, minimum_size=1000)


@app.post("/report")
async def receive_report(
    rep: ReportPayload,
    creds: HTTPAuthorizationCredentials | None = Depends(bearer),
) -> ReportResp:
    agent_id = str(rep.agent_id)
    if agent_id not in registered_agents:
        raise HTTPException(status_code=403, detail="agent not registered — run go-fim setup first")

    row = queries.find_agent_by_id(conn, agent_id)
    if row["api_token"] is not None:
        if creds is None or creds.credentials != row["api_token"]:
            raise HTTPException(status_code=401, detail="invalid or missing API token")

    now = datetime.now(timezone.utc).isoformat()
    with conn:
        queries.refresh_agent(conn, agent_id, rep.agent_name, rep.scan_path, now)
        queries.save_report(
            conn,
            agent_id,
            rep.timestamp.isoformat(),
            rep.model_dump_json(),
            RETENTION_N,
        )

    return ReportResp()


@app.get("/", response_class=HTMLResponse)
async def index(request: Request):
    agents = [agent_view(r) for r in queries.list_agents(conn)]
    return templates.TemplateResponse(request, "index.html", {"agents": agents})


# /agents/new must be registered BEFORE /agents/{agent_id}, otherwise FastAPI
# matches "new" as the agent_id parameter.
@app.get("/agents/new", response_class=HTMLResponse)
async def add_agent_form(request: Request):
    return templates.TemplateResponse(request, "agent_new.html", {})


@app.post("/agents/new", response_class=HTMLResponse)
async def add_agent_submit(
    request: Request,
    agent_name: str = Form(..., min_length=1, max_length=255),
    scan_path: str = Form(..., min_length=1),
    excludes: str = Form(""),
):
    exclude_lines = [ln.strip() for ln in excludes.splitlines() if ln.strip()]
    token = auth.mint_setup_token(agent_name, scan_path, exclude_lines)

    # Public URL precedence: explicit env var > the host the form was served
    # from. The Caddy demo terminates TLS, so request.base_url is already the
    # external URL (caddy forwards X-Forwarded-* and Starlette honors it when
    # uvicorn is launched with --proxy-headers).
    public_url = os.environ.get("GOFIM_PUBLIC_URL") or str(request.base_url).rstrip("/")

    return templates.TemplateResponse(
        request,
        "agent_bootstrap.html",
        {
            "agent_name": agent_name,
            "scan_path": scan_path,
            "excludes": exclude_lines,
            "server_url": public_url,
            "token": token,
        },
    )


@app.post("/api/setup")
async def api_setup(
    req: SetupReq,
    creds: HTTPAuthorizationCredentials | None = Depends(bearer),
) -> SetupResp:
    if creds is None or creds.scheme.lower() != "bearer":
        raise HTTPException(status_code=401, detail="missing bearer token")

    try:
        claims = auth.verify_setup_token(creds.credentials)
    except jwt.InvalidTokenError as e:
        raise HTTPException(status_code=401, detail=f"invalid token: {e}")

    agent_id = str(req.agent_id)
    now = datetime.now(timezone.utc).isoformat()

    with conn:
        api_token = queries.register_agent(
            conn, agent_id, claims["agent_name"], claims["scan_path"], now
        )
        if api_token:
            registered_agents.add(agent_id)
    if api_token is None:
        raise HTTPException(
            status_code=409,
            detail=f"agent_id {agent_id} is already registered",
        )

    return SetupResp(
        agent_id=agent_id,
        agent_name=claims["agent_name"],
        scan_path=claims["scan_path"],
        api_token=api_token,
    )


def _agent_context(agent_id: str) -> dict:
    row = queries.find_agent_by_id(conn, agent_id)
    if row is None:
        raise HTTPException(status_code=404, detail="agent not found")
    report_rows = queries.list_agent_reports(conn, row["id"], DASHBOARD_N)
    return {"agent": agent_view(row), "reports": [report_view(rr) for rr in report_rows]}


@app.get("/agents/{agent_id}", response_class=HTMLResponse)
async def agent_detail(request: Request, agent_id: str):
    return templates.TemplateResponse(request, "agent.html", _agent_context(agent_id))


@app.get("/partial/agents", response_class=HTMLResponse)
async def partial_agents(request: Request):
    agents = [agent_view(r) for r in queries.list_agents(conn)]
    return templates.TemplateResponse(
        request, "partials/agents_table.html", {"agents": agents}
    )


@app.get("/partial/agent/{agent_id}/reports", response_class=HTMLResponse)
async def partial_agent_reports(request: Request, agent_id: str):
    return templates.TemplateResponse(request, "partials/agent_reports.html", _agent_context(agent_id))
