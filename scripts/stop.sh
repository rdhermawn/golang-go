#!/bin/bash

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PID_FILE="$SCRIPT_DIR/refine-monitor.pid"

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
    pkill -f "refine-monitor" 2>/dev/null
    echo "Stopped (by name)"
fi
