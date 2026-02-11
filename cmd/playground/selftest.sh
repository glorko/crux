#!/bin/bash
# Self-test script for terminal spawning
# This script tests that we can:
# 1. Spawn a terminal with a worker
# 2. Communicate via FIFO
# 3. Clean shutdown

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
CRUX_DIR="$(dirname "$(dirname "$SCRIPT_DIR")")"

echo "╔════════════════════════════════════════════════╗"
echo "║     Terminal Spawning Self-Test                ║"
echo "╚════════════════════════════════════════════════╝"
echo ""

# Build worker
echo "Step 1: Building worker..."
cd "$CRUX_DIR"
go build -o /tmp/crux-worker ./cmd/playground/worker
echo "  ✅ Worker built"

# Clean up any previous test files
rm -f /tmp/crux-selftest.pipe /tmp/crux-selftest.pid

# Determine terminal to use
TERMINAL="${CRUX_TERMINAL:-ghostty}"
echo ""
echo "Step 2: Testing $TERMINAL terminal spawning..."

# Spawn based on terminal type
case "$TERMINAL" in
    ghostty)
        echo "  Spawning Ghostty window..."
        open -na Ghostty.app --args --title="SelfTest" -e /bin/sh -c 'exec /tmp/crux-worker selftest'
        ;;
    terminal)
        echo "  Spawning Terminal.app window..."
        osascript -e 'tell application "Terminal" to do script "/tmp/crux-worker selftest"'
        ;;
    *)
        echo "  ❌ Unknown terminal: $TERMINAL"
        exit 1
        ;;
esac

# Wait for worker to start
echo "  Waiting for worker to initialize..."
sleep 3

# Check if PID file exists
if [ -f /tmp/crux-selftest.pid ]; then
    PID=$(cat /tmp/crux-selftest.pid)
    echo "  ✅ Worker started (PID: $PID)"
else
    echo "  ❌ Worker PID file not found!"
    echo "  Check if a terminal window appeared and if the worker is running."
    exit 1
fi

# Check if process is alive
if kill -0 "$PID" 2>/dev/null; then
    echo "  ✅ Worker process is alive"
else
    echo "  ❌ Worker process is not running!"
    exit 1
fi

# Test FIFO communication
echo ""
echo "Step 3: Testing FIFO communication..."

if [ -p /tmp/crux-selftest.pipe ]; then
    echo "  ✅ FIFO pipe exists"
else
    echo "  ❌ FIFO pipe not found!"
    exit 1
fi

# Send reload command
echo "  Sending 'r' (reload) command..."
echo "r" > /tmp/crux-selftest.pipe
sleep 0.5
echo "  ✅ Reload command sent (check terminal window for output)"

# Send status command
echo "  Sending 's' (status) command..."
echo "s" > /tmp/crux-selftest.pipe
sleep 0.5
echo "  ✅ Status command sent (check terminal window for output)"

# Test graceful shutdown
echo ""
echo "Step 4: Testing graceful shutdown..."
echo "  Sending 'q' (quit) command..."
echo "q" > /tmp/crux-selftest.pipe
sleep 1

# Check if process exited
if kill -0 "$PID" 2>/dev/null; then
    echo "  ⚠️  Worker still running, sending SIGTERM..."
    kill "$PID" 2>/dev/null || true
    sleep 1
fi

if kill -0 "$PID" 2>/dev/null; then
    echo "  ❌ Worker did not exit!"
    kill -9 "$PID" 2>/dev/null || true
else
    echo "  ✅ Worker exited gracefully"
fi

# Cleanup
rm -f /tmp/crux-selftest.pipe /tmp/crux-selftest.pid

echo ""
echo "╔════════════════════════════════════════════════╗"
echo "║     ✅ Self-Test Complete!                     ║"
echo "╚════════════════════════════════════════════════╝"
echo ""
echo "If you saw a terminal window appear with worker output,"
echo "and the reload/status messages appeared in that window,"
echo "then terminal spawning is working correctly!"
echo ""
echo "Next: Run the full playground with:"
echo "  CRUX_TERMINAL=$TERMINAL /tmp/crux-playground"
