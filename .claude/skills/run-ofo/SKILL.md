---
name: run-ofo
description: Build, launch, smoke-test, and screenshot the ofo blog (Go/Gin/SQLite). Use when asked to run, start, launch, screenshot, or drive the app.
---

# Run ofo Blog

Go + Gin blog engine with SQLite (pure Go, no CGO). Serves SSR HTML pages,
RSS feed, static files, and an admin panel at `/admin`.

**All paths relative to repo root** (`E:\go-project\ofo` on Windows,
`<repo>` on Linux).

## Prerequisites

```bash
# Linux — one-time:
apt-get install -y golang-go curl
# Windows — install Go from https://go.dev/dl/

# Browser screenshots (optional):
npm install @playwright/test && npx playwright install chromium
```

## Build

```bash
go build -o ofo .
```

No CGO needed — `modernc.org/sqlite` is a pure-Go SQLite driver.
Builds on Linux, macOS, and Windows.

## Run (agent path)

Use the smoke driver — it builds, launches, tests every endpoint, and cleans up:

```bash
# Full smoke test (build → launch → curl all endpoints → screenshots → stop)
bash .claude/skills/run-ofo/smoke.sh all
```

Or step-by-step:

```bash
# 1. Build & launch in background, wait for ready
bash .claude/skills/run-ofo/smoke.sh start

# 2. Curl smoke test (all public endpoints + admin login + RSS)
bash .claude/skills/run-ofo/smoke.sh test

# 3. Take screenshots (requires playwright)
bash .claude/skills/run-ofo/smoke.sh screenshot

# 4. Stop the server
bash .claude/skills/run-ofo/smoke.sh stop
```

### Config

All via env vars (defaults shown):

| Var | Default |
|-----|---------|
| `PORT` | `8080` |
| `DB_PATH` | `db/log.db` |
| `BLOG_TITLE` | `骑自行车` |
| `BLOG_AUTHOR` | `青头儿包` |
| `BASE_URL` | `http://localhost:8080` |
| `ADMIN_PASSWORD` | `admin123` |

```bash
# Example: custom port
PORT=3000 bash .claude/skills/run-ofo/smoke.sh all
```

### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Home page (paginated post list) |
| GET | `/post/:slug` | Single post |
| GET | `/category/:slug` | Category filter |
| GET | `/tag/:slug` | Tag filter |
| GET | `/about` | About page |
| GET | `/rss.xml` (or `/feed.xml`) | RSS feed |
| GET | `/admin/login` | Admin login page |
| POST | `/admin/login` | Admin login (form: `password=...`) |
| GET | `/admin/` | Admin dashboard (auth required) |
| GET | `/static/*` | Static files (CSS, JS, uploads) |

Screenshots land in `static/screenshots/`.

## Run (human path)

```bash
go build -o ofo . && ./ofo
# → http://localhost:8080
# Ctrl-C to stop
```

On first run, the database is auto-created and seeded with 5 sample posts
("Building REST APIs with Go and Gin", "Understanding Goroutines and Channels",
etc.) with categories (Go, DevOps, Architecture, JavaScript, Tools) and tags.

Delete `db/` to reset — the next launch re-seeds.

## Test

```bash
go test ./...          # unit tests (if any)
go vet ./...           # static analysis
```

## Direct invocation

For PRs touching handler/model/middleware logic without needing the full server:

```go
// Import and call directly in a test or script:
import "ofo/handlers"
import "ofo/models"
import "ofo/config"

cfg := config.Load()
db, _ := sql.Open("sqlite", ":memory:")
// ... run migrations, seed, test handler logic
```

## Gotchas

- **DB auto-creates directories** — `main.go` creates `db/` and `static/uploads/` on startup. Run from repo root or the paths won't match.
- **Seed is idempotent** — checks `SELECT COUNT(*) FROM posts`; won't duplicate data on restart. Delete `db/log.db` to force re-seed.
- **Admin auth is cookie-based** — POST `/admin/login` with `password=<value>` sets a session cookie. Curl tests need `-c cookie.jar` / `-b cookie.jar`.
- **RSS returns XML, not HTML** — `Accept: application/xml` header not required; it always returns XML.
- **Port 8080 must be free** — the smoke script auto-kills existing processes on that port, but check manually if you're not using the script.
- **Template changes need rebuild** — `go build` recompiles the binary, but templates are loaded from disk at startup. No rebuild needed for template-only changes if the server restarts.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `go: cannot find main module` | Run from repo root: `cd <repo>` |
| `port 8080 already in use` | `pkill -f ofo` or use `PORT=3000` |
| DB locked / "database is locked" | WAL mode handles concurrent reads; kill other ofo processes |
| `panic: ... templates/*.html` not found | Run from repo root (templates are loaded relative to cwd) |
| Screenshot blank / 404 | Check the server is running: `curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/` |
| `playwright: command not found` | `npx playwright install chromium` or skip screenshots with `bash smoke.sh test` |
