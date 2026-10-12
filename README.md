# Защищённая оверлейная сеть передачи данных на основе пиринговых протоколов

**Курсовая работа. Этапы 1–2.**  
Транспорт, кадрирование, криптографическая идентичность, Kademlia DHT,
bootstrap.

Реализован узел P2P-сети на Go: TCP-транспорт с собственным форматом
кадров, устойчивая идентичность на Ed25519, ограниченная таблица
маршрутизации Kademlia, операции `PING` и `FIND_NODE`, присоединение к
сети через bootstrap-узел и итеративный поиск узлов.

---

## Содержание

- [Требования](#требования)
- [Быстрый старт](#быстрый-старт)
- [Структура проекта](#структура-проекта)
- [Конфигурация узла](#конфигурация-узла)
- [Режимы запуска](#режимы-запуска)
- [Сценарии экспериментов](#сценарии-экспериментов)
- [Тесты](#тесты)
- [Артефакты эксперимента](#артефакты-эксперимента)
- [Ограничения этапа 2](#ограничения-этапа-2)
- [Документация](#документация)

---

## Требования

- **Go 1.22+**
- **Linux** (проверено на Zorin OS)
- **jq** — для обработки JSON-выводов (`sudo apt install -y jq`)
- **netcat** — для smoke-тестов (`sudo apt install -y netcat-openbsd`)
- **goimports** (опционально, для форматирования):
  ```bash
  go install golang.org/x/tools/cmd/goimports@latest
  export PATH="$HOME/go/bin:$PATH"
  ```

---

## Быстрый старт

Полный цикл: сборка, запуск сети из 21 узла, контрольные lookup,
остановка.

```bash
# 1. Клонировать репозиторий и перейти в каталог
cd p2p-node

# 2. Сгенерировать конфиги для обеих топологий
bash scripts/gen_configs.sh star
bash scripts/gen_configs.sh ring

# 3. Собрать бинарник
go build -o bin/node ./cmd/node

# 4. Запустить сеть из 21 узла по схеме star
bash scripts/run_star.sh

# 5. Снять сводку по таблицам маршрутизации
./bin/node --dump-tables --state-dir ./state/star | jq .

# 6. Выполнить приёмочный сценарий (10 lookup + отказ seed)
bash scripts/demo.sh star

# 7. Остановить все узлы
bash scripts/stop_all.sh
```

Аналогично для ring: заменить `star` на `ring`.

---

## Структура проекта

```text
cmd/node/           точка входа: daemon, --lookup, --dump-tables
internal/
  transport/        TCP-сервер и клиент, пул соединений, кадрирование
  protocol/         типы сообщений, payload-структуры, JSON-кодек
  identity/         Ed25519-ключ, вычисление NodeID
  routing/          XOR, Contact, k-bucket, RoutingTable
  rpc/              клиент RPC, корреляция по request_id, диспетчер
  node/             конфиг, bootstrap, lookup, обработчики, dump, signals
  logging/          обёртка над log/slog
scripts/            запуск стенда, генерация конфигов, демонстрация
configs/{star,ring}/  по 21 yaml-конфигу на топологию
docs/               PROTOCOL.md, ARCHITECTURE.md, experiments/
testdata/           независимые тест-векторы для XOR-метрики
state/              каталоги состояния узлов (в .gitignore)
results/            логи и результаты экспериментов (в .gitignore)
```

---

## Конфигурация узла

Все параметры узла задаются в YAML. Пример `configs/star/node-05.yaml`:

```yaml
node_state_dir: ./state/star/node-05
listen_host: 127.0.0.1
listen_port: 9105
bootstrap_peers: ["127.0.0.1:9101"]
node_id_bits: 256
k_bucket_size: 4
alpha: 3
connect_timeout_ms: 3000
read_timeout_ms: 5000
ping_timeout_ms: 5000
max_frame_payload: 65536
protocol_version: 1
log_level: INFO
advertise_host: 127.0.0.1
```

**Ключевые параметры:**

| Параметр             | Назначение                                                       |
|----------------------|------------------------------------------------------------------|
| `node_state_dir`     | Каталог состояния: `identity.key`, `table.json`                  |
| `listen_host/port`   | Адрес TCP-сервера; `port: 0` → ОС выберет свободный порт         |
| `bootstrap_peers`    | Начальные контакты; пусто только у seed-узла                     |
| `k_bucket_size`      | Ёмкость bucket, `4` для продвинутого уровня                      |
| `alpha`              | Параллелизм lookup, `3`                                          |
| `connect_timeout_ms` | Тайм-аут установления TCP                                        |
| `read_timeout_ms`    | Тайм-аут ожидания кадра                                          |
| `ping_timeout_ms`    | Тайм-аут ответа PONG                                             |
| `max_frame_payload`  | 65536, проверяется до аллокации                                  |
| `log_level`          | `DEBUG` / `INFO` / `WARN` / `ERROR`                              |
| `advertise_host`     | Адрес, сообщаемый другим узлам (нужен при NAT или `:0`)          |

Генерация конфигов для всех 21 узла:

```bash
bash scripts/gen_configs.sh star   # или ring
```

---

## Режимы запуска

### 1. Daemon (обычный узел)

```bash
./bin/node --config configs/star/node-01.yaml
```

Узел стартует, выполняет bootstrap (если указаны peers), ждёт сигнала
`SIGTERM`/`SIGINT`.

### 2. Разовая операция lookup

```bash
./bin/node --config configs/star/node-05.yaml \
    --lookup <target-node-id-hex> \
    --wait-ms 4000
```

Узел стартует, присоединяется к сети, ждёт сходимости `wait-ms`,
выполняет `FIND_NODE` lookup для указанного `NodeID` и печатает
результат в JSON в stdout, после чего завершается.

**Пример вывода:**

```json
{
  "lookup_id": "8d83695f",
  "initiator_id": "b2eea69e",
  "target_hex": "f4e40ef0f8bb0b6fbc1b26c7f00ed30507fd8b1f176634d865110f5625d5c861",
  "success": true,
  "rpcs": 1,
  "iterations": 1,
  "timeouts_hex": null,
  "duration_ms": 0,
  "closest_hex": [
    "f4e40ef0f8bb0b6fbc1b26c7f00ed30507fd8b1f176634d865110f5625d5c861"
  ],
  "table_size_before": 1,
  "table_size_after": 1,
  "timestamp": "2026-09-28T07:51:03Z"
}
```

Логи узла идут в **stderr**, JSON-результат — в **stdout**. Это позволяет
перенаправлять их независимо:

```bash
./bin/node --config ... --lookup HEX > result.json 2> node.log
```

### 3. Сводка по таблицам маршрутизации

```bash
./bin/node --dump-tables --state-dir ./state/star | jq .
```

Агрегирует все `state/<topo>/node-*/table.json` и выводит:

```json
{
  "count": 21,
  "total_size": 175,
  "avg_size": 8.33,
  "nodes": [
    {
      "node_short": "3b45a1f5",
      "self_id": "3b45a1f579a0f665a4116f21cadca55bb8e2123c3bf44e9ccf4467972fd16176",
      "table_size": 12,
      "path": ".../state/star/node-01/table.json"
    }
  ]
}
```

### 4. Дамп таблицы по сигналу

```bash
# Долгоживущий узел в фоне
./bin/node --config configs/star/node-05.yaml &

# По SIGUSR1 узел сохраняет таблицу в state/<node>/table.json
kill -USR1 $!
```

Используется в `scripts/demo.sh` для сбора состояния со всех узлов.

---

## Сценарии экспериментов

### Star bootstrap

```bash
bash scripts/run_star.sh   # 21 узел, все подключаются к node-01
```

### Ring bootstrap

```bash
bash scripts/run_ring.sh   # node-XX подключается к node-(XX-1)
```

### Демонстрация

```bash
bash scripts/demo.sh star    # или ring
```

Скрипт выполняет:

1. Проверку, что все узлы живы.
2. `SIGUSR1` всем узлам → дамп таблиц в `state/`.
3. Сводную статистику по таблицам.
4. 10 контрольных `FIND_NODE` lookup от эфемерных узлов (отдельный
   `node_state_dir`, `listen_port: 0`) — гарантирует, что цель
   отсутствовала в таблице инициатора.
5. Остановку seed-узла (`node-01`).
6. Контрольный lookup между ранее присоединившимися узлами — проверка
   независимости от bootstrap (требование T2.14).

Результаты сохраняются в `results/<topo>/lookup/`.

### Остановка

```bash
bash scripts/stop_all.sh
```

---

## Тесты

```bash
# Все тесты с race detector
go test ./... -race -count=1

# Только конкретный пакет
go test ./internal/routing/... -race -count=1 -v

# Статический анализ
go vet ./...

# Форматирование
goimports -w $(find . -name '*.go' -not -path './vendor/*')
```

**Покрытие требований раздела 24 ТЗ:**

| Тест                                  | Файл                                     |
|---------------------------------------|------------------------------------------|
| T1.1–T1.9  кадрирование               | `internal/transport/frame_test.go`       |
| T1.10      корреляция `request_id`    | `internal/rpc/rpc_test.go`               |
| T2.1–T2.4  идентичность               | `internal/identity/identity_test.go`     |
| T2.5       XOR-метрика                | `internal/routing/xor_test.go`           |
| T2.6–T2.9  k-bucket                   | `internal/routing/table_test.go`         |
| T2.10      PING/PONG                  | `internal/node/handlers_test.go`         |
| T2.11      FIND_NODE сортировка       | `internal/node/handlers_test.go`         |
| T2.12–T2.14 bootstrap и lookup        | `internal/node/lookup_test.go`           |

---

## Артефакты эксперимента

```
docs/experiments/
├── star/
│   ├── table-summary.json           // сводка таблиц после сходимости
│   ├── lookup-1.json … lookup-10.json  // 10 контрольных lookup
│   └── lookup-after-seed-down.json  // lookup без bootstrap-узла
└── ring/
    └── (аналогично)
```

Ключевые показатели (N=21, K=4, α=3):

| Метрика                       | Star    | Ring    |
|-------------------------------|--------:|--------:|
| Средний размер таблицы        | 8.33    | 9.90    |
| Максимальный размер           | 13      | 14      |
| Минимальный размер            | 4       | 6       |
| Невырожденность (< N-1 = 20)  | 21/21   | 21/21   |
| Контрольных lookup успешных   | 10/10   | 10/10   |
| Диапазон итераций lookup      | 2–5     | 2–3     |

Подробнее — в `docs/ARCHITECTURE.md`, §10.

---

## Ограничения этапа 2

**Канал не защищён.** TLS/AKE, аутентификация транспортных сообщений и
E2E-защита прикладных данных появятся на этапе 3. Публичный ключ
передаётся в составе `Contact` без подписи. Это означает, что **этап 2
не устраняет MITM-угрозу** и не обеспечивает конфиденциальность.

**Что уже защищено:**

- `NodeID` связан с публичным ключом через `SHA-256` — подмена без
  приватного ключа невозможна;
- контакт с тем же `NodeID`, но иным публичным ключом отклоняется;
- закрытые ключи не покидают каталог состояния (права `0600/0700`);
- `payload_length` проверяется до выделения буфера;
- нарушение протокола закрывает соединение.

**Ограничения DHT:** нет `STORE`/`FIND_VALUE`, TTL, репликации,
периодического refresh. Всё это — этап 3.

---

## Документация

| Документ                     | Содержание                                                    |
|------------------------------|---------------------------------------------------------------|
| `docs/PROTOCOL.md`           | Формат кадра, типы сообщений, схемы payload, XOR-метрика,     |
|                              | алгоритм lookup, bootstrap, правило anti-spoofing             |
| `docs/ARCHITECTURE.md`       | Модули, обоснование инженерных решений, диаграммы, ограничения этапа 2, результаты экспериментов, план этапа 3   |
| `docs/experiments/{star,ring}` | Сырые данные контрольных прогонов                          |

---

## Лицензия

Курсовой проект. Использование в учебных целях.

