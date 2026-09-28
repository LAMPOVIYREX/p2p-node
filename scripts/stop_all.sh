#!/usr/bin/env bash
# Останавливает все запущенные узлы всех топологий.
set -uo pipefail

for topo in star ring; do
    dir="./state/$topo"
    [ -d "$dir" ] || continue
    for pidfile in "$dir"/node-*.pid; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
        fi
        rm -f "$pidfile"
    done
    echo "stopped topology: $topo"
done