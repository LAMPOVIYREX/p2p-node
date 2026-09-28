#!/usr/bin/env bash
# Приёмочный сценарий раздела 25.2.
# Запускать ПОСЛЕ scripts/run_star.sh и после того, как сеть сходилась.
set -euo pipefail

N=21
BIN=./bin/node
CFGDIR=./configs
STATEDIR=./state
RESULTS=./results/star
LOGDIR="$RESULTS/logs"
OUT="$RESULTS/lookup"

mkdir -p "$OUT/tables"

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
    echo "мало живых узлов, сначала запусти run_star.sh" >&2
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
"$BIN" --dump-tables | tee "$OUT/table-summary.json"

echo
echo "==> выбор инициатора и целей для контрольных lookup"

# Собираем список self_id всех узлов из дампов.
mapfile -t all_ids < <(for f in "$OUT"/tables/node-*.json; do
    jq -r '.self_id' "$f"
done)
if [ "${#all_ids[@]}" -lt 2 ]; then
    echo "недостаточно узлов для lookup" >&2
    exit 1
fi

# Инициатор — node-02.
INITIATOR_CFG="$CFGDIR/node-02.yaml"

# Цели — 10 узлов, начиная с node-05, чтобы не совпали с инициатором.
targets=()
for i in $(seq -w 5 $((4 + 10))); do
    f="$STATEDIR/node-$i/table.json"
    [ -f "$f" ] || continue
    targets+=("$(jq -r '.self_id' "$f")")
done

echo "==> 10 контрольных lookup"
k=0
for tgt in "${targets[@]}"; do
    k=$((k + 1))
    echo "lookup #$k: initiator=node-02 target=${tgt:0:8}…"
    "$BIN" --config "$INITIATOR_CFG" \
        --lookup "$tgt" \
        --wait-ms 3000 \
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
"$BIN" --config "$CFGDIR/node-03.yaml" \
    --lookup "$tgt" \
    --wait-ms 1000 \
    > "$OUT/lookup-after-seed-down.json" 2> "$OUT/lookup-after-seed-down.err"

echo
echo "==> собрано:"
echo "  таблицы:     $OUT/tables/"
echo "  сводка:      $OUT/table-summary.json"
echo "  lookups:     $OUT/lookup-*.json"
echo "  после seed:  $OUT/lookup-after-seed-down.json"
echo "готово"