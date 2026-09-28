#!/usr/bin/env bash
# Приёмочный сценарий раздела 25.2 (этап 2).
# Запускать ПОСЛЕ scripts/run_star.sh, когда сеть уже поднялась.
set -euo pipefail

TOP0=star
LOGDIR="./results/$TOP0/logs"
OUT="./results/$TOP0/lookup"
mkdir -p "$OUT"

echo "==> Экспорт таблиц маршрутизации всех узлов"
# Требует, чтобы узел поддерживал SIGUSR1 → dump в state/node-XX/table.json.
# Если такой обработки нет — экспорт выполняется вручную через
# узел-админ команду (см. ARCHITECTURE.md, раздел 5).
for i in $(seq -w 1 21); do
    pidfile="./state/$TOP0/node-$i.pid"
    [ -f "$pidfile" ] || continue
    pid=$(cat "$pidfile")
    kill -USR1 "$pid" 2>/dev/null || true
done
sleep 1

echo "==> Ожидание сходимости (30 с)"
sleep 30

echo "==> Сбор выгрузок таблиц"
# Копирование дампов, созданных по SIGUSR1.
mkdir -p "$OUT/tables"
for i in $(seq -w 1 21); do
    src="./state/$TOP0/node-$i/table.json"
    [ -f "$src" ] && cp "$src" "$OUT/tables/node-$i.json"
done

echo "==> Контрольные lookup (10 шт., цель заведомо неизвестна инициатору)"
# Инициатор и цель выбираются скриптом; сам lookup инициируется на узле
# через админ-команду (см. ARCHITECTURE.md).
for k in $(seq 1 10); do
    # Здесь должна вызываться ваша узел-команда, например:
    #   ./bin/node --config configs/$TOP0/node-XX.yaml --lookup <target-hex>
    # Результат пишется в $OUT/lookup-$k.json
    echo "lookup #$k -> (см. $OUT/lookup-$k.json)"
done

echo "==> Остановка bootstrap-узла (node-01)"
pidfile="./state/$TOP0/node-01.pid"
if [ -f "$pidfile" ]; then
    kill "$(cat "$pidfile")" || true
    rm -f "$pidfile"
    echo "node-01 остановлен"
fi

echo "==> Контрольный lookup между двумя оставшимися узлами"
# ... вызов lookup между node-02 и node-03 без bootstrap-узла

echo "Готово. Результаты в $OUT/"