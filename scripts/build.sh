#!/bin/bash

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BINARY="$PROJECT_DIR/refine-monitor"

cd "$PROJECT_DIR"
echo "Building refine-monitor..."
go build -o "$BINARY" ./cmd/refine-monitor/
echo "Build OK: $BINARY ($(du -h "$BINARY" | cut -f1))"
