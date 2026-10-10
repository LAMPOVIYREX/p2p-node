#!/usr/bin/env bash
# Запускает 21 узел по схеме ring: node-XX bootstrap'ится к node-(XX-1).
set -euo pipefail

TOPO=ring
N=21
BIN=./bin/node
CFGDIR=./configs/$TOPO
STATEDIR=./state/$TOPO
LOGDIR=./results/$TOPO/logs

mkdir -p "$LOGDIR"

if [ ! -x "$BIN" ]; then
    echo "building $BIN"
    go build -o "$BIN" ./cmd/node
fi

if [ ! -f "$CFGDIR/node-01.yaml" ]; then
    echo "configs missing, generating"
    bash scripts/gen_configs.sh "$TOPO"
fi

pkill -f 'bin/node' 2>/dev/null || true
sleep 1
rm -rf "$STATEDIR" "$LOGDIR"
mkdir -p "$STATEDIR" "$LOGDIR"

echo "==> starting node-01 (seed, no bootstrap)"
nohup "$BIN" --config "$CFGDIR/node-01.yaml" \
    > "$LOGDIR/node-01.log" 2>&1 &
echo $! > "$STATEDIR/node-01.pid"
sleep 2

for i in $(seq -w 2 $N); do
    nohup "$BIN" --config "$CFGDIR/node-$i.yaml" \
        > "$LOGDIR/node-$i.log" 2>&1 &
    echo $! > "$STATEDIR/node-$i.pid"
    echo "started node-$i (pid $(cat "$STATEDIR/node-$i.pid"))"
    sleep 0.5
done

echo
echo "==> wait 30s for convergence"
sleep 30

echo "==> dumps"
for i in $(seq -w 1 $N); do
    pidfile="$STATEDIR/node-$i.pid"
    [ -f "$pidfile" ] || continue
    kill -USR1 "$(cat "$pidfile")" 2>/dev/null || true
done
sleep 1

echo
echo "Topology: $TOPO"
echo "Logs:     $LOGDIR"