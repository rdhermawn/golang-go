#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY="$PROJECT_DIR/refine-monitor"
LOG_FILE="$PROJECT_DIR/logs/monitorar_refino.log"
PID_FILE="$PROJECT_DIR/refine-monitor.pid"

if [ -f "$PID_FILE" ]; then
    OLD_PID=$(cat "$PID_FILE")
    if kill -0 "$OLD_PID" 2>/dev/null; then
        echo "Already running (PID: $OLD_PID)"
        exit 1
    fi
    rm -f "$PID_FILE"
fi

echo "Building latest binary..."
mkdir -p "$PROJECT_DIR/logs"
cd "$PROJECT_DIR"
go build -o "$BINARY" ./cmd/refine-monitor/

nohup "$BINARY" > "$LOG_FILE" 2>&1 &
echo $! > "$PID_FILE"

echo "Started (PID: $(cat $PID_FILE))"
