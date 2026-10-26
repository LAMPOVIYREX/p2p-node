#!/usr/bin/env bash
# Приёмочный сценарий раздела 25.2 для указанной топологии.
# Usage: demo.sh <topology>   # star | ring
#
# Требует, чтобы сеть уже была запущена scripts/run_<topology>.sh.
# Каждый контрольный lookup выполняется отдельным эфемерным узлом
# (изолированный state_dir, listen_port: 0). Если цель случайно
# оказалась в таблице инициатора (тривиальный lookup), попытка
# повторяется с новым эфемерным узлом — до `max_attempts` раз.
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
MAX_RETRIES=5

case "$TOPO" in
    star|ring) ;;
    *) echo "unknown topology: $TOPO (use star or ring)" >&2; exit 1 ;;
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
    echo "мало живых узлов, сначала запусти scripts/run_${TOPO}.sh" >&2
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
"$BIN" --dump-tables --state-dir "$STATEDIR" > "$OUT/table-summary.json"
jq '{count, avg_size, min_size: ([.nodes[].table_size]|min), max_size: ([.nodes[].table_size]|max)}' \
    "$OUT/table-summary.json"

echo "==> выбор целей для контрольных lookup"

# Временный конфиг эфемерного узла-инициатора.
# listen_port: 0 → ОС выдаёт свободный порт, конфликтов с работающими
# узлами нет. node_state_dir уникален ($RANDOM дважды) — гарантирует
# свежий NodeID и свежую таблицу для каждого запуска.
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

# run_lookup_with_retry: выполняет lookup до тех пор, пока цель не
# окажется отсутствующей в таблице инициатора до запуска. Возвращает 0,
# если получен нетривиальный lookup; 1 — если все попытки дали
# тривиальный результат (тогда в stderr пишется предупреждение).
run_lookup_with_retry() {
    local target="$1"
    local bport="$2"
    local out="$3"
    local max_attempts="${4:-$MAX_RETRIES}"
    local attempt flag cfg

    for attempt in $(seq 1 "$max_attempts"); do
        cfg="$TMPDIR_DEMO/cfg-$RANDOM-$RANDOM.yaml"
        make_lookup_cfg "$cfg" "$bport"
        "$BIN" --config "$cfg" \
            --lookup "$target" \
            --wait-ms 4000 \
            > "$out" 2> "${out%.json}.err"

        flag=$(jq -r '.target_in_table_before' "$out" 2>/dev/null || echo "null")
        if [ "$flag" = "false" ]; then
            [ "$attempt" -gt 1 ] && echo "    → нетривиальный с попытки $attempt"
            return 0
        fi
        echo "    попытка $attempt/$max_attempts: цель в таблице, повтор с новым узлом"
    done

    echo "    ПРЕДУПРЕЖДЕНИЕ: за $max_attempts попыток цель всегда в таблице" >&2
    return 1
}

# Цели — 10 узлов, начиная с node-05, чтобы не совпадали с seed
# и первыми запущенными узлами.
targets=()
for i in $(seq -w 5 $((4 + 10))); do
    f="$STATEDIR/node-$i/table.json"
    [ -f "$f" ] || continue
    targets+=("$(jq -r '.self_id' "$f")")
done

if [ "${#targets[@]}" -lt 10 ]; then
    echo "предупреждение: найдено только ${#targets[@]} целей" >&2
fi

echo "==> ${#targets[@]} контрольных lookup"
nontrivial=0
k=0
for tgt in "${targets[@]}"; do
    k=$((k + 1))
    echo "lookup #$k: target=${tgt:0:8}…"
    if run_lookup_with_retry "$tgt" 9101 "$OUT/lookup-$k.json" "$MAX_RETRIES"; then
        nontrivial=$((nontrivial + 1))
    fi
done

echo
echo "==> итог по контрольным lookup:"
echo "нетривиальных (цель отсутствовала у инициатора): $nontrivial / ${#targets[@]}"

echo
echo "==> сводка по RPC и итерациям:"
for i in $(seq 1 "${#targets[@]}"); do
    f="$OUT/lookup-$i.json"
    [ -f "$f" ] || continue
    jq -c '{i:'"$i"', success, rpcs, iterations, in_table_before: .target_in_table_before}' "$f"
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

echo "==> контрольный lookup без seed (bootstrap через node-03)"
tgt="${targets[0]}"
if run_lookup_with_retry "$tgt" 9103 "$OUT/lookup-after-seed-down.json" "$MAX_RETRIES"; then
    target_found=$(jq -r '.target_found' "$OUT/lookup-after-seed-down.json")
    jq -c '{success, target_found, rpcs, iterations, in_table_before: .target_in_table_before}' \
        "$OUT/lookup-after-seed-down.json"
    
    if [ "$target_found" = "true" ]; then
        echo "   ЦЕЛЬ НАЙДЕНА: ${tgt:0:8}… присутствует в closest_hex"
    else
        echo "   ОШИБКА: цель ${tgt:0:8}… ОТСУТСТВУЕТ в closest_hex" >&2
        exit 1
    fi
else
    echo "ПРЕДУПРЕЖДЕНИЕ: контрольный lookup без seed остался тривиальным" >&2
fi

echo
echo "==> собрано:"
echo "  таблицы:     $OUT/tables/"
echo "  сводка:      $OUT/table-summary.json"
echo "  lookups:     $OUT/lookup-*.json"
echo "  после seed:  $OUT/lookup-after-seed-down.json"