# go-fim

[![CI](https://github.com/Jimzical/go-fim/actions/workflows/ci.yml/badge.svg)](https://github.com/Jimzical/go-fim/actions/workflows/ci.yml)
[![Release](https://github.com/Jimzical/go-fim/actions/workflows/release.yml/badge.svg)](https://github.com/Jimzical/go-fim/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/Jimzical/go-fim)](https://goreportcard.com/report/github.com/Jimzical/go-fim)

A file integrity monitor (FIM): Go agents walk a filesystem, hash files, diff against the last snapshot, and POST change reports to a Python FastAPI control plane. The server stores per-agent history in SQLite and serves a live Jinja+HTMX dashboard.

## How it works

- Parallel filesystem walk via `fastwalk`; 8-worker hash pool
- Snapshot persisted in bbolt; each run diffs against the prior one (created / modified / deleted)
- Reports written locally to `<path>/.gofim/history/` (rolling, last 10 kept); POSTed to the server with Bearer auth if `server_url` is set
- On POST failure the report is queued as `.unsent`; the next run replays pending files oldest-first before sending the fresh one
- Agents must register via `--setup` before the server accepts their reports; registration issues a long-lived API token stored in bbolt
- Server stores up to 50 reports per agent and renders a dashboard that auto-refreshes every 10s via HTMX

> **Note:** Hash is currently a placeholder (`sha256(size:mtime)`) — no file reads. Swapping to real content hashing is a one-function change in `internal/hasher/hasher.go`.

## Getting started

### Standalone (no server)

Useful for local diffing without a control plane.

```bash
go build -o go-fim ./cmd/go-fim

# Zero-config: scans the current directory, stores state in ./.gofim/
./go-fim -local

# Or with a config file:
cat > gofim.yml <<EOF
path: ~/projects/myapp
exclude:
  - '^\.git$'
  - '^node_modules$'
EOF

./go-fim          # first run builds the snapshot; subsequent runs diff against it
./go-fim -v       # verbose output
```

### With the control plane

**1. Start the server**

```bash
cd server
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
pip install -r requirements.txt
uvicorn app:app --reload --port 8000
```

Dashboard at `http://localhost:8000/`.

**2. Register an agent**

Open the dashboard, click **Add agent**, fill in the agent name, scan path, and any excludes. The server renders a bootstrap command — copy and run it on the agent machine:

```bash
# The dashboard generates this for you:
go-fim --setup <JWT> -c gofim.yml
```

This calls `POST /api/setup`, creates the agent row in SQLite, and stores a long-lived API token in the agent's bbolt database. The token is sent automatically on every subsequent `/report`.

**3. Schedule scans**

```bash
# crontab entry — runs every 5 minutes
*/5 * * * * /usr/local/bin/go-fim -c /etc/gofim.yml
```

## Configuration (`gofim.yml`)

```yaml
path: ~/projects/myapp     # required — directory to scan
agent_name: prod-web-01    # required when server_url is set — display label

server_url: https://fim.example.com   # omit for standalone mode (no POST)
exclude:                              # regexes matched against directory basename
  - '^\.git$'
  - '^node_modules$'
  - '^\.cache$'

insecure_skip_verify: false   # disable TLS verification (dev / self-signed certs only)

# agent_id: <uuid>            # optional — pin a stable UUID instead of using the bbolt one
```

`path` supports `~` expansion and is resolved to an absolute path at load time. The bbolt snapshot and report history are always stored under `<path>/.gofim/` — not configurable. `exclude` patterns are Go regexes matched against directory **basenames**, not full paths — matching a directory skips it entirely.

## CLI reference

```
go-fim [-c gofim.yml] [-v] [-local]   # cron-driven scan
go-fim [-c gofim.yml] --setup <JWT>   # one-shot registration handshake
```

| Flag | Default | Description |
|------|---------|-------------|
| `-c` | `gofim.yml` | Path to config file |
| `-v` | `false` | Force verbose output |
| `-local` | `false` | Run without a config file, scanning cwd with no server |
| `--setup` | — | Register this agent using the JWT from the dashboard, then exit |

## Layout

```
├── cmd/go-fim/main.go              # entry point — flag parsing only
├── internal/
│   ├── agent/
│   │   ├── agent.go                # Run: scan + report loop
│   │   ├── setup.go                # Setup: registration handshake
│   │   └── util.go                 # shared helpers
│   ├── config/config.go            # YAML loader, regex compile, ~ expansion
│   ├── walker/walker.go            # parallel fastwalk, exclude pruning
│   ├── hasher/hasher.go            # 8-worker fan-out/fan-in hash pool
│   ├── store/
│   │   ├── meta.go                 # agent UUID + API token persisted in bbolt
│   │   └── snapshot.go             # diff engine + batched bbolt writes
│   ├── client/client.go            # HTTP wrapper; ErrUnreachable classification
│   ├── logger/logger.go            # slog wrapper
│   └── report/
│       ├── report.go               # JSON payload builder + rolling history writer
│       └── queue.go                # .unsent FIFO queue (cap 10)
└── server/
    ├── app.py                      # FastAPI routes
    ├── auth.py                     # JWT mint / verify for setup tokens
    ├── db.py                       # SQLite init
    ├── models.py                   # Pydantic wire shapes
    ├── queries.py                  # all SQL
    ├── dashboard.py                # Row → template dict, relative time, freshness
    └── templates/                  # Jinja2 + HTMX partials
```

## Multi-agent demo (Docker)

Runs three agents against different host paths plus the server, all behind Caddy (TLS):

```bash
docker compose -f demo/docker-compose.yml up --build
```

Each agent scans a host directory mounted read-only, with a named volume mounted at `<path>/.gofim/` so the snapshot and UUID survive restarts. Reports are POSTed to the server every 30 seconds.

The compose file bind-mounts paths under `$HOME/Developer/...` — edit `demo/docker-compose.yml` and the matching `demo/*.gofim.yml` files if you're running on a different machine. Dashboard at `http://localhost:8000/`.

## Installation

### Quick Install (Recommended)

**Linux / macOS:**
```bash
curl -sSL https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.sh | bash
sudo mv go-fim /usr/local/bin/
```

**Windows (PowerShell):**
```powershell
iwr -useb https://raw.githubusercontent.com/Jimzical/go-fim/main/scripts/install.ps1 | iex
# Then move go-fim.exe to a directory in your PATH
```

### From source

```bash
go install github.com/Jimzical/go-fim/cmd/go-fim@latest
```

### Manual download from GitHub Releases

Download a pre-built binary for your platform from the [releases page](https://github.com/Jimzical/go-fim/releases).

**Linux / macOS:**
```bash
VERSION=1.0.0

# Linux (amd64)
curl -Lo go-fim.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_linux_amd64.tar.gz
tar -xzf go-fim.tar.gz && chmod +x go-fim
sudo mv go-fim /usr/local/bin/

# macOS (Apple Silicon)
curl -Lo go-fim.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_darwin_arm64.tar.gz
tar -xzf go-fim.tar.gz && chmod +x go-fim
sudo mv go-fim /usr/local/bin/

# macOS (Intel)
curl -Lo go-fim.tar.gz https://github.com/Jimzical/go-fim/releases/download/v${VERSION}/go-fim_${VERSION}_darwin_amd64.tar.gz
tar -xzf go-fim.tar.gz && chmod +x go-fim
sudo mv go-fim /usr/local/bin/
```

**Windows:**
1. Download the `.zip` file for your architecture from the [releases page](https://github.com/Jimzical/go-fim/releases)
2. Extract `go-fim.exe`
3. Add to a directory in your PATH

## Releasing

Tag-driven via GoReleaser. Creates binaries for Linux / macOS / Windows × amd64 / arm64 and a changelog from conventional commits.

```bash
git tag v1.2.3
git push origin v1.2.3
```

## Development

```bash
# Agent
go vet ./...
gofmt -l .          # CI fails if this prints anything; run gofmt -w . to fix
go mod tidy

# Server
cd server
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
pip install -r requirements.txt
uvicorn app:app --reload --port 8000
```

CI runs `golangci-lint`, `gofmt -l .`, `go mod tidy` drift check, `go vet`, and `go build`.
