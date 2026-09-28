#!/usr/bin/env bash
# Останавливает все узлы, запущенные run_star.sh или run_ring.sh.
set -uo pipefail

STATEDIR=./state

if [ ! -d "$STATEDIR" ]; then
    echo "no state dir, nothing to stop"
    exit 0
fi

count=0
for pidfile in "$STATEDIR"/node-*.pid; do
    [ -f "$pidfile" ] || continue
    pid=$(cat "$pidfile")
    if kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
        count=$((count + 1))
    fi
    rm -f "$pidfile"
done

echo "stopped $count nodes"