#!/usr/bin/env bash
# Приёмочный сценарий раздела 25.2 для указанной топологии.
# Usage: demo.sh <topology>   # star | ring
set -euo pipefail

TOPO="${1:-star}"
N=21
BIN=./bin/node
CFGDIR=./configs/$TOPO
STATEDIR=./state/$TOPO
RESULTS=./results/$TOPO
LOGDIR="$RESULTS/logs"
OUT="$RESULTS/lookup"
TMPDIR_DEMO="$RESULTS/tmp"

case "$TOPO" in
    star) SEED_PORT=9101 ;;
    ring) SEED_PORT=9101 ;;
    *) echo "unknown topology: $TOPO" >&2; exit 1 ;;
esac

mkdir -p "$OUT/tables" "$TMPDIR_DEMO"

echo "==> topology: $TOPO"
echo "==> проверка, что сеть работает"
alive=0
for i in $(seq -w 1 $N); do
    pidfile="$STATEDIR/node-$i.pid"
    [ -f "$pidfile" ] || continue
    if kill -0 "$(cat "$pidfile")" 2>/dev/null; then
        alive=$((alive + 1))
    fi
done
echo "живых узлов: $alive / $N"
if [ "$alive" -lt 5 ]; then
    echo "мало живых узлов, сначала запусти run_${TOPO}.sh" >&2
    exit 1
fi

echo "==> SIGUSR1 всем узлам + сбор таблиц"
for i in $(seq -w 1 $N); do
    pidfile="$STATEDIR/node-$i.pid"
    [ -f "$pidfile" ] || continue
    kill -USR1 "$(cat "$pidfile")" 2>/dev/null || true
done
sleep 2

for i in $(seq -w 1 $N); do
    src="$STATEDIR/node-$i/table.json"
    [ -f "$src" ] && cp "$src" "$OUT/tables/node-$i.json"
done

echo "==> сводная статистика таблиц"
"$BIN" --dump-tables --state-dir "$STATEDIR" | tee "$OUT/table-summary.json" > /dev/null

echo "==> выбор инициатора и целей"
make_lookup_cfg() {
    local out="$1"
    local bport="$2"
    cat > "$out" <<EOF
node_state_dir: $TMPDIR_DEMO/state-$RANDOM-$RANDOM
listen_host: 127.0.0.1
listen_port: 0
bootstrap_peers: ["127.0.0.1:$bport"]
node_id_bits: 256
k_bucket_size: 4
alpha: 3
connect_timeout_ms: 3000
read_timeout_ms: 5000
ping_timeout_ms: 5000
max_frame_payload: 65536
protocol_version: 1
log_level: WARN
advertise_host: 127.0.0.1
EOF
}

targets=()
for i in $(seq -w 5 $((4 + 10))); do
    f="$STATEDIR/node-$i/table.json"
    [ -f "$f" ] || continue
    targets+=("$(jq -r '.self_id' "$f")")
done

echo "==> ${#targets[@]} контрольных lookup"
k=0
for tgt in "${targets[@]}"; do
    k=$((k + 1))
    cfg="$TMPDIR_DEMO/lookup-$k.yaml"
    make_lookup_cfg "$cfg" "$SEED_PORT"
    echo "lookup #$k: target=${tgt:0:8}…"
    "$BIN" --config "$cfg" \
        --lookup "$tgt" \
        --wait-ms 4000 \
        > "$OUT/lookup-$k.json" 2> "$OUT/lookup-$k.err"
done

echo
echo "==> остановка seed (node-01)"
pidfile="$STATEDIR/node-01.pid"
if [ -f "$pidfile" ]; then
    kill "$(cat "$pidfile")" 2>/dev/null || true
    rm -f "$pidfile"
    echo "node-01 остановлен"
fi
sleep 2

echo "==> контрольный lookup без seed"
tgt="${targets[0]}"
cfg="$TMPDIR_DEMO/lookup-after-seed-down.yaml"
make_lookup_cfg "$cfg" 9103
"$BIN" --config "$cfg" \
    --lookup "$tgt" \
    --wait-ms 4000 \
    > "$OUT/lookup-after-seed-down.json" 2> "$OUT/lookup-after-seed-down.err"

echo
echo "==> собрано:"
echo "  таблицы:     $OUT/tables/"
echo "  сводка:      $OUT/table-summary.json"
echo "  lookups:     $OUT/lookup-*.json"
echo "  после seed:  $OUT/lookup-after-seed-down.json"