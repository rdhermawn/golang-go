#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PID_FILE="$PROJECT_DIR/refine-monitor.pid"
BINARY="$PROJECT_DIR/refine-monitor"

if [ -f "$PID_FILE" ]; then
    PID=$(cat "$PID_FILE")
    if kill -0 "$PID" 2>/dev/null; then
        kill "$PID"
        echo "Stopped (PID: $PID)"
    else
        echo "Not running (stale PID: $PID)"
    fi
    rm -f "$PID_FILE"
else
    if pkill -f "$BINARY" 2>/dev/null; then
        echo "Stopped (by binary path)"
    else
        echo "Not running"
    fi
fi
