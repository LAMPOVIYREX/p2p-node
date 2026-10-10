#!/usr/bin/env bash
# Останавливает все узлы обеих топологий.
set -uo pipefail

count=0
for topo in star ring; do
    dir="./state/$topo"
    [ -d "$dir" ] || continue
    for pidfile in "$dir"/node-*.pid; do
        [ -f "$pidfile" ] || continue
        pid=$(cat "$pidfile")
        if kill -0 "$pid" 2>/dev/null; then
            kill "$pid" 2>/dev/null || true
            count=$((count + 1))
        fi
        rm -f "$pidfile"
    done
done

echo "stopped $count nodes"