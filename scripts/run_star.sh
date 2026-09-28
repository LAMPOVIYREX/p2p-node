#!/usr/bin/env bash
set -euo pipefail

TOPO=star
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

# seed
nohup "$BIN" --config "configs/$TOPO/node-01.yaml" \
    > "$LOGDIR/node-01.log" 2>&1 &
echo $! > "$STATEDIR/node-01.pid"
echo "started node-01 (seed, no bootstrap)"

sleep 2

for i in $(seq -w 2 $N); do
    nohup "$BIN" --config "configs/$TOPO/node-$i.yaml" \
        > "$LOGDIR/node-$i.log" 2>&1 &
    echo $! > "$STATEDIR/node-$i.pid"
    echo "started node-$i"
    sleep 0.3
done

echo
echo "Topology: $TOPO"
echo "Logs:     $LOGDIR"
echo "PIDs:     $STATEDIR/*.pid"