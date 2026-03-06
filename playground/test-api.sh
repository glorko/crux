#!/usr/bin/env bash
# Test crux HTTP API and new features (kill tab, reload, version).
# Run from repo root or from crux/playground. Requires wezterm and a display.

set -e

CRUX_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLAYGROUND_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$CRUX_DIR"

echo "=== Building crux ==="
go build -o /tmp/crux-test ./cmd/playground
go build -o /tmp/crux-mcp-test ./cmd/mcp
/tmp/crux-test --version
echo ""

# Kill any existing crux on 9876 so we can bind
if curl -s -o /dev/null -w "%{http_code}" http://localhost:9876/health 2>/dev/null | grep -q 200; then
  echo "Stopping existing crux on 9876..."
  curl -s -X POST http://localhost:9876/stop 2>/dev/null || true
  sleep 2
fi

echo "=== Starting crux with playground config (background) ==="
cd "$PLAYGROUND_DIR"
/tmp/crux-test -c config.yaml &
CRUX_PID=$!
cd "$CRUX_DIR"

# Wait for API to be up
echo "Waiting for API (http://localhost:9876)..."
for i in $(seq 1 30); do
  if curl -s -o /dev/null -w "%{http_code}" http://localhost:9876/health 2>/dev/null | grep -q 200; then
    echo "  API ready after ${i}s"
    break
  fi
  if ! kill -0 $CRUX_PID 2>/dev/null; then
    echo "  crux process exited early"
    exit 1
  fi
  sleep 1
done

if ! curl -s -o /dev/null -w "%{http_code}" http://localhost:9876/health 2>/dev/null | grep -q 200; then
  echo "  Timeout waiting for API"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi

echo ""
echo "=== 1. GET /tabs ==="
TABS=$(curl -s http://localhost:9876/tabs)
echo "$TABS"
if ! echo "$TABS" | grep -q '"tabs"'; then
  echo "  FAIL: expected tabs in response"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi
echo "  OK"
echo ""

echo "=== 2. POST /stop/backend (kill tab) ==="
STOP_RESULT=$(curl -s -X POST http://localhost:9876/stop/backend -H "Content-Type: application/json")
echo "$STOP_RESULT"
if echo "$STOP_RESULT" | grep -q '"success":true'; then
  echo "  OK: backend tab killed"
else
  echo "  FAIL: stop returned: $STOP_RESULT"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi
sleep 1
echo ""

echo "=== 3. GET /tabs (after kill) ==="
curl -s http://localhost:9876/tabs | head -20
echo ""
echo ""

echo "=== 4. POST /start-one/backend (restart) ==="
START_RESULT=$(curl -s -X POST http://localhost:9876/start-one/backend -H "Content-Type: application/json")
echo "$START_RESULT"
if echo "$START_RESULT" | grep -q '"success":true'; then
  echo "  OK: backend started"
else
  echo "  FAIL: start-one result: $START_RESULT"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi
sleep 1
echo ""

echo "=== 5. Reload backend (POST /stop/backend then POST /start-one/backend) ==="
RELOAD_KILL=$(curl -s -X POST http://localhost:9876/stop/backend)
RELOAD_START=$(curl -s -X POST http://localhost:9876/start-one/backend)
if echo "$RELOAD_KILL" | grep -q '"success":true' && echo "$RELOAD_START" | grep -q '"success":true'; then
  echo "  OK: reload (kill + start_one) succeeded"
else
  echo "  FAIL: reload kill=$RELOAD_KILL start=$RELOAD_START"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi
sleep 1
echo ""

echo "=== 6. POST /send/backend (hot reload key) ==="
SEND_RESULT=$(curl -s -X POST http://localhost:9876/send/backend -H "Content-Type: application/json" -d '{"text":"r"}')
echo "$SEND_RESULT"
if echo "$SEND_RESULT" | grep -q '"success":true'; then
  echo "  OK: send r to backend"
else
  echo "  FAIL: send result: $SEND_RESULT"
  kill $CRUX_PID 2>/dev/null || true
  exit 1
fi
echo ""

echo "=== 7. Shutdown (POST /stop) ==="
curl -s -X POST http://localhost:9876/stop
sleep 2
if kill -0 $CRUX_PID 2>/dev/null; then
  kill $CRUX_PID 2>/dev/null || true
fi
echo "  OK"
echo ""
echo "=== All API checks done ==="
