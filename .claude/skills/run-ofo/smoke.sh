#!/usr/bin/env bash
# ============================================================
# ofo blog — smoke-test driver
# Build, launch, test, screenshot, stop.
#
# Usage:
#   bash smoke.sh all          # full cycle
#   bash smoke.sh start        # build + background launch
#   bash smoke.sh test         # curl smoke (server must be up)
#   bash smoke.sh screenshot   # playwright screenshots
#   bash smoke.sh stop         # kill server
#
# Config via env (with defaults):
#   PORT=8080  DB_PATH=db/log.db  ADMIN_PASSWORD=admin123
# ============================================================
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
PORT="${PORT:-8080}"
BASE_URL="${BASE_URL:-http://localhost:$PORT}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin123}"
SCREENSHOT_DIR="$REPO_ROOT/static/screenshots"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; exit 1; }
info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

# ---- helpers ----
_curl() {
    local code
    code=$(curl -s -o /dev/null -w "%{http_code}" "$@")
    echo "$code"
}

_ready() {
    for i in $(seq 1 20); do
        if curl -s -o /dev/null "$BASE_URL/" 2>/dev/null; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

# ---- start ----
do_start() {
    cd "$REPO_ROOT"

    # kill existing process on port
    if command -v lsof >/dev/null 2>&1; then
        lsof -ti:"$PORT" 2>/dev/null | xargs kill -9 2>/dev/null || true
    fi
    # Windows fallback — run via bash but skip if no lsof
    sleep 1

    info "Building..."
    go build -o ofo.exe -ldflags="-s -w" . 2>&1 || go build -o ofo . 2>&1

    info "Starting on port $PORT..."
    ./ofo.exe 2>&1 &
    SERVER_PID=$!
    echo "$SERVER_PID" > /tmp/ofo-smoke.pid

    if _ready; then
        pass "Server is up on $BASE_URL"
    else
        fail "Server did not start within 10s"
    fi
}

# ---- test ----
do_test() {
    cd "$REPO_ROOT"
    local ok=0 total=0

    check() {
        local label="$1" code="$2" expected="$3"
        total=$((total + 1))
        if [ "$code" = "$expected" ]; then
            pass "$label ($code)"
            ok=$((ok + 1))
        else
            echo -e "${RED}[FAIL]${NC} $label (got $code, expected $expected)"
        fi
    }

    info "Smoke-testing endpoints..."

    check "GET /"              "$(_curl "$BASE_URL/")"                      200
    check "GET /about"         "$(_curl "$BASE_URL/about")"                 200
    check "GET /rss.xml"       "$(_curl "$BASE_URL/rss.xml")"              200
    check "GET /feed.xml"      "$(_curl "$BASE_URL/feed.xml")"             200
    check "GET /admin/login"   "$(_curl "$BASE_URL/admin/login")"          200
    check "GET /static/css/style.css"  "$(_curl "$BASE_URL/static/css/style.css")"  200
    check "GET /static/js/main.js"     "$(_curl "$BASE_URL/static/js/main.js")"     200
    check "GET /nonexistent"   "$(_curl "$BASE_URL/nonexistent")"          404

    # Admin login
    local login_code
    login_code=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/admin/login" -d "password=$ADMIN_PASSWORD")
    check "POST /admin/login"  "$login_code"                               302

    # Admin page without cookie → redirect
    local admin_code
    admin_code=$(curl -s -o /dev/null -w "%{http_code}" "$BASE_URL/admin/")
    check "GET /admin/ (no cookie)" "$admin_code"                          302

    # RSS content check
    local rss_title
    rss_title=$(curl -s "$BASE_URL/rss.xml" | grep -o '<title>[^<]*</title>' | head -1)
    if [ -n "$rss_title" ]; then
        pass "RSS has title ($rss_title)"
    else
        fail "RSS title missing"
    fi

    # Home page title check
    if curl -s "$BASE_URL/" | grep -q '<title>骑自行车</title>'; then
        pass "Home page title correct"
    else
        fail "Home page title wrong/missing"
    fi

    echo ""
    echo "========================================"
    if [ "$ok" -eq "$total" ]; then
        echo -e "${GREEN}All $total checks passed${NC}"
    else
        echo -e "${RED}$ok/$total checks passed${NC}"
        exit 1
    fi
    echo "========================================"
}

# ---- screenshot ----
do_screenshot() {
    cd "$REPO_ROOT"
    mkdir -p "$SCREENSHOT_DIR"

    info "Taking screenshots with playwright..."

    local pages=(
        "/:home"
        "/about:about"
        "/admin/login:admin-login"
    )

    for entry in "${pages[@]}"; do
        local path="${entry%%:*}"
        local name="${entry##*:}"
        info "  $BASE_URL$path → $SCREENSHOT_DIR/$name.png"
        npx playwright screenshot --browser chromium "$BASE_URL$path" "$SCREENSHOT_DIR/$name.png" 2>&1
    done

    # Verify at least one non-empty screenshot
    local size
    size=$(stat -c%s "$SCREENSHOT_DIR/home.png" 2>/dev/null || stat -f%z "$SCREENSHOT_DIR/home.png" 2>/dev/null || echo "0")
    if [ "$size" -gt 10000 ]; then
        pass "Screenshots saved ($SCREENSHOT_DIR/), home.png = ${size} bytes"
    else
        fail "Screenshot home.png too small (${size} bytes) — may be blank"
    fi
}

# ---- stop ----
do_stop() {
    info "Stopping server..."
    if [ -f /tmp/ofo-smoke.pid ]; then
        kill "$(cat /tmp/ofo-smoke.pid)" 2>/dev/null || true
        rm -f /tmp/ofo-smoke.pid
    fi
    # Broad cleanup
    if command -v pkill >/dev/null 2>&1; then
        pkill -f "ofo" 2>/dev/null || true
    fi
    sleep 1
    pass "Server stopped"
}

# ---- dispatch ----
case "${1:-all}" in
    start)      do_start ;;
    test)       do_test ;;
    screenshot) do_screenshot ;;
    stop)       do_stop ;;
    all)
        do_start
        do_test
        # screenshots optional — skip if playwright not installed
        if npx playwright --version >/dev/null 2>&1; then
            do_screenshot
        else
            info "Skipping screenshots (playwright not installed)"
            info "Install: npm install @playwright/test && npx playwright install chromium"
        fi
        do_stop
        ;;
    *)
        echo "Usage: bash smoke.sh {all|start|test|screenshot|stop}"
        echo "  all         — build → start → test → screenshot → stop (default)"
        echo "  start       — build + background launch"
        echo "  test        — curl smoke test (requires running server)"
        echo "  screenshot  — playwright screenshots (requires running server)"
        echo "  stop        — kill server"
        exit 1
        ;;
esac
