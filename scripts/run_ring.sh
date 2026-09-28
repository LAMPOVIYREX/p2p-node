#!/usr/bin/env bash
set -euo pipefail

TOPO=ring
N=21
BIN=./bin/node
LOGDIR="./results/$TOPO/logs"
STATEDIR="./state/$TOPO"

mkdir -p "$LOGDIR" "$STATEDIR"

if [ ! -x "$BIN" ]; then
    go build -o "$BIN" ./cmd/node
fi

if [ ! -f "configs/$TOPO/node-01.yaml" ]; then
    bash scripts/gen_configs.sh "$TOPO"
fi

for i in $(seq -w 1 $N); do
    nohup "$BIN" --config "configs/$TOPO/node-$i.yaml" \
        > "$LOGDIR/node-$i.log" 2>&1 &
    echo $! > "$STATEDIR/node-$i.pid"
    echo "started node-$i"
    sleep 0.5
done

echo
echo "Topology: $TOPO (ring)"
echo "Logs:     $LOGDIR"