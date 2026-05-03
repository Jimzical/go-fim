"""FastAPI control plane for go-fim agents.

Endpoints:
  POST /report                          → upsert agent + save the scan diff.
  GET  /                                → agent list with freshness dots.
  GET  /agents/{agent_id}               → timeline of last DASHBOARD_N reports.
  GET  /partial/agents                  → HTMX partial: agents table only.
  GET  /partial/agent/{agent_id}/reports → HTMX partial: reports cards only.

Identity is the agent_id (UUID): names and scan paths come from each agent's
YAML and are display-only. No /register handshake — the first /report from a
new agent_id implicitly creates the row.
"""

from contextlib import asynccontextmanager
from datetime import datetime, timezone
from pathlib import Path

from fastapi import FastAPI, HTTPException, Request
from fastapi.middleware.gzip import GZipMiddleware
from fastapi.responses import HTMLResponse
from fastapi.templating import Jinja2Templates

from . import queries
from .dashboard import agent_view, report_view
from .db import conn, init_db
from .models import ReportPayload, ReportResp

RETENTION_N = 50
DASHBOARD_N = 10

templates = Jinja2Templates(directory=Path(__file__).parent / "templates")


@asynccontextmanager
async def lifespan(app: FastAPI):
    init_db()
    yield


app = FastAPI(lifespan=lifespan, title="go-fim control plane", docs_url=None, redoc_url=None)
app.add_middleware(GZipMiddleware, minimum_size=1000)


@app.post("/report")
async def receive_report(rep: ReportPayload) -> ReportResp:
    agent_id = str(rep.agent_id)
    now = datetime.now(timezone.utc).isoformat()
    queries.upsert_agent(conn, agent_id, rep.agent_name, rep.scan_path, now)
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


@app.get("/agents/{agent_id}", response_class=HTMLResponse)
async def agent_detail(request: Request, agent_id: str):
    row = queries.find_agent_by_id(conn, agent_id)
    if row is None:
        raise HTTPException(status_code=404, detail="agent not found")

    report_rows = queries.list_agent_reports(conn, row["id"], DASHBOARD_N)
    return templates.TemplateResponse(
        request,
        "agent.html",
        {
            "agent": agent_view(row),
            "reports": [report_view(rr) for rr in report_rows],
        },
    )


@app.get("/partial/agents", response_class=HTMLResponse)
async def partial_agents(request: Request):
    agents = [agent_view(r) for r in queries.list_agents(conn)]
    return templates.TemplateResponse(
        request, "partials/agents_table.html", {"agents": agents}
    )


@app.get("/partial/agent/{agent_id}/reports", response_class=HTMLResponse)
async def partial_agent_reports(request: Request, agent_id: str):
    row = queries.find_agent_by_id(conn, agent_id)
    if row is None:
        raise HTTPException(status_code=404, detail="agent not found")

    report_rows = queries.list_agent_reports(conn, row["id"], DASHBOARD_N)
    return templates.TemplateResponse(
        request,
        "partials/agent_reports.html",
        {
            "agent": agent_view(row),
            "reports": [report_view(rr) for rr in report_rows],
        },
    )
