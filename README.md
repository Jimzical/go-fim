# go-fim

[![CI](https://github.com/Jimzical/go-fim/actions/workflows/ci.yml/badge.svg)](https://github.com/Jimzical/go-fim/actions/workflows/ci.yml)
[![Release](https://github.com/Jimzical/go-fim/actions/workflows/release.yml/badge.svg)](https://github.com/Jimzical/go-fim/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Jimzical/go-fim)](https://goreportcard.com/report/github.com/Jimzical/go-fim)

A file integrity monitor (FIM): Go agents walk a filesystem, hash files, diff against the last snapshot, and POST change reports to a Python FastAPI control plane. The server stores per-agent history in SQLite and serves a live Jinja+HTMX dashboard.

## What it does

- Parallel filesystem walk via `fastwalk`; 8-worker SHA256 hasher pool
- Snapshot persisted in bbolt; each run diffs against the prior one (created / modified / deleted)
- Reports written locally to `history/` (rolling, last 10 kept); POSTed to the control plane if `server_url` is set
- On POST failure the report is renamed `.unsent`; the next run replays pending files oldest-first before sending the fresh one
- Server lazily creates the agent row on the first `/report` (no registration handshake); stores up to 50 reports per agent and renders a dashboard that auto-refreshes every 10s via HTMX
- Agent identity is a stable UUID stored in bbolt; `agent_name` and `scan_path` are operator-supplied display fields sent on every report

> Hash is currently a placeholder (`sha256(size:mtime)`) — no file reads. Swapping to real content hashing is a one-function change in `internal/hasher/hasher.go`.

## Layout

```
├── cmd/go-fim/main.go         # entry point
├── internal/
│   ├── config/config.go       # YAML loader + regex compile + ~ expansion
│   ├── walker/walker.go       # parallel walker
│   ├── hasher/hasher.go       # 8-worker fan-out/fan-in
│   ├── store/
│   │   ├── meta.go            # agent UUID persisted in bbolt
│   │   └── snapshot.go        # diff + batched bbolt writes
│   ├── client/client.go       # HTTP /report; ErrUnreachable classification
│   └── report/
│       ├── report.go          # JSON payload builder + rolling history writer
│       └── queue.go           # .unsent FIFO queue (cap 10)
└── server/
    ├── app.py                 # routes
    ├── db.py                  # SQLite init
    ├── models.py              # Pydantic shapes
    ├── queries.py             # all SQL
    ├── dashboard.py           # view helpers
    └── templates/             # Jinja2 + HTMX partials
```

## Build & run

```bash
go build -o go-fim ./cmd/go-fim

./go-fim                       # uses ./gofim.yml
./go-fim -v                    # verbose
./go-fim -c ~/other.yml        # alternate config
```

### `gofim.yml`

```yaml
agent_name: my-agent           # required when server_url is set — display label
path: ~/projects/foo           # required — directory to scan
exclude:                       # optional — regexes on directory basename
  - '^\.git$'
  - '^node_modules$'
verbose: false
db: ./snapshot.db
history_dir: ./history
server_url: http://localhost:8000   # omit for standalone mode (no POST)
```

### Control plane

```bash
cd server
pipenv install                 # first time only
pipenv run uvicorn app:app --reload --port 8000
```

Dashboard at `http://localhost:8000/`. Agent detail pages are at `/agents/{agent_id}`. SQLite store auto-created at `server/server.db`.

### Multi-agent demo (Docker)

Runs three agents against different host paths plus the server, all in Docker:

```bash
docker compose -f demo/docker-compose.yml up --build
```

Each agent scans a host directory mounted read-only, writes its bbolt snapshot and history to a named volume (so the UUID survives restarts), and POSTs to the server every 30 seconds. The compose file expects `$HOME/Developer/Learning/file-checker`, `$HOME/Developer/Learning`, and `$HOME/Developer/Personal` to exist — edit `demo/docker-compose.yml` and the corresponding `demo/*.gofim.yml` files to point at paths that exist on your machine.

Dashboard at `http://localhost:8000/`. The stack appears as **go-fim-demo** in Docker Desktop.

## Installation

### From Source

```bash
go install github.com/Jimzical/go-fim/cmd/go-fim@latest
```

### From GitHub Releases

Download a release binary for your platform (replace `VERSION` with the release tag without the leading `v`, for example `1.0.0`):

```bash
VERSION=1.0.0

# Linux (amd64)
curl -Lo go-fim_${VERSION}_linux_amd64.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_linux_amd64.tar.gz
tar -xzf go-fim_${VERSION}_linux_amd64.tar.gz
chmod +x go-fim

# Linux (arm64)
curl -Lo go-fim_${VERSION}_linux_arm64.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_linux_arm64.tar.gz
tar -xzf go-fim_${VERSION}_linux_arm64.tar.gz
chmod +x go-fim

# macOS (Apple Silicon)
curl -Lo go-fim_${VERSION}_darwin_arm64.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_darwin_arm64.tar.gz
tar -xzf go-fim_${VERSION}_darwin_arm64.tar.gz
chmod +x go-fim

# macOS (Intel)
curl -Lo go-fim_${VERSION}_darwin_amd64.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_darwin_amd64.tar.gz
tar -xzf go-fim_${VERSION}_darwin_amd64.tar.gz
chmod +x go-fim
```

For Windows, download the `.zip` file from the [releases page](https://github.com/Jimzical/go-fim/releases).

## Releasing

This project uses [GoReleaser](https://goreleaser.com/) for automated releases. To create a new release:

```bash
# Create and push a version tag
git tag v1.0.0
git push origin v1.0.0
```

This triggers the release workflow which:
1. Builds binaries for Linux, macOS, and Windows (amd64 + arm64)
2. Creates a GitHub Release with auto-generated changelog
3. Uploads all binaries as release assets

# Future work

- [x] Setup Github Actions for basic tests and release builds (possibly just a simple lint + test action for now)
- [ ] Maybe Graceful shutdown for agents?
- [ ] Break down the main.go with a runner and setup command for automated config generation and db init?
- [ ] JWT and agent adding button on the dashboard for auth and onboarding, maybe with a simple shared secret for now
- [ ] Possible sqlite backup strategy for server.db?

