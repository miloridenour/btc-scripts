#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/config.env" ]; then
    source "$SCRIPT_DIR/config.env"
fi

HOST="${MAPPING_BOT_URL:-http://localhost:8000}"

usage() {
    echo "Usage: $0 <instruction>"
    echo ""
    echo "Registers an instruction with the mapping bot."
    echo ""
    echo "Environment:"
    echo "  MAPPING_BOT_URL  Bot URL (default: http://localhost:8000)"
    exit 1
}

if [ $# -lt 1 ]; then
    usage
fi

INSTRUCTION="$1"

curl -s -w "\nHTTP %{http_code}\n" \
    -X POST "$HOST" \
    -H "Content-Type: application/json" \
    -d "{\"instruction\": \"$INSTRUCTION\"}"
