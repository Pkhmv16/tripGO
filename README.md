# TripGO — HTTP API поездок

Лабораторные работы 1 и 2: HTTP API, PostgreSQL, архитектура по слоям, тесты и observability.

## Подготовка окружения

Команды выполняются из корня репозитория в Linux или WSL. Нужны Go версии из `go.mod`, GNU Make, Docker, `tripgoctl` и `golangci-lint` v2. Для тестов с `-race` нужен C-компилятор, например GCC.

Поднимите окружение:

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
```

Окружение лабораторной работы 2 задано в `environment.toml`. `tripgoctl` генерирует `.env` с актуальными адресами PostgreSQL и коллектора. Создайте рядом в корне репозитория `.env.local` и добавьте в него дополнительные настройки сервиса:

```dotenv
HTTP_READ_TIMEOUT=10s
HTTP_READ_HEADER_TIMEOUT=5s
HTTP_WRITE_TIMEOUT=15s
HTTP_IDLE_TIMEOUT=60s
HTTP_MAX_BODY_BYTES=1048576
DATABASE_ROLLBACK_TIMEOUT=3s
IDEMPOTENCY_TTL=24h
```

Сохраняйте `DATABASE_URL` и `OTEL_EXPORTER_OTLP_ENDPOINT`, выданные для текущего окружения: адрес PostgreSQL из `.env.example` может относиться к другой лабораторной работе. `.env.example` служит справочником полного набора настроек; копировать его поверх сгенерированного `.env` не нужно.

Makefile читает сначала `.env`, затем `.env.local`: значения из `.env.local` имеют приоритет. При повторной генерации `.env` дополнительные настройки остаются в `.env.local`.

Примените миграции и запустите сервис:

```bash
make migrate
make run
```

`make run` передаёт настройки из обоих файлов в окружение процесса. Сам сервис читает переменные окружения; для прямого `go run ./cmd/service` их нужно предварительно экспортировать.

## Конфигурация

Полный пример находится в [`.env.example`](.env.example).

| Переменная | Назначение | Пример |
|---|---|---|
| `HTTP_ADDR` | Адрес HTTP-сервера | `:8080` |
| `HTTP_READ_TIMEOUT` | Таймаут чтения запроса | `10s` |
| `HTTP_READ_HEADER_TIMEOUT` | Таймаут чтения заголовков | `5s` |
| `HTTP_WRITE_TIMEOUT` | Таймаут записи ответа | `15s` |
| `HTTP_IDLE_TIMEOUT` | Таймаут простаивающего HTTP-соединения | `60s` |
| `HTTP_MAX_BODY_BYTES` | Максимальный размер тела создания поездки, байты | `1048576` |
| `LOG_LEVEL` | `debug`, `info`, `warn` или `error` | `info` |
| `SHUTDOWN_TIMEOUT` | Бюджет остановки HTTP, экспорта телеметрии и закрытия пула | `10s` |
| `DATABASE_URL` | Строка подключения PostgreSQL | Из окружения `tripgoctl` |
| `DATABASE_MAX_CONNS` | Максимальное число соединений пула | `10` |
| `DATABASE_MIN_CONNS` | Минимальное число соединений пула | `2` |
| `DATABASE_MAX_CONN_LIFETIME` | Максимальное время жизни соединения | `30m` |
| `DATABASE_CONNECT_TIMEOUT` | Таймаут подключения и стартовой проверки БД | `5s` |
| `DATABASE_QUERY_TIMEOUT` | Таймаут операции репозитория, `BEGIN`, `COMMIT` и readiness | `3s` |
| `DATABASE_ROLLBACK_TIMEOUT` | Таймаут отката транзакции | `3s` |
| `IDEMPOTENCY_TTL` | Срок действия ключа идемпотентности | `24h` |
| `OTEL_SERVICE_NAME` | Имя сервиса в телеметрии | `trip-service` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Адрес коллектора | Из окружения `tripgoctl` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Протокол экспорта | `http/protobuf` или `grpc` |
| `OTEL_RESOURCE_ATTRIBUTES` | Атрибуты ресурса | `service.namespace=tripgo,deployment.environment=local` |
| `OTEL_TRACES_SAMPLER` | Сэмплирование трейсов | `parentbased_always_on` |
| `OTEL_TRACES_SAMPLER_ARG` | Доля трейсов для `traceidratio` и `parentbased_traceidratio` | `1` |

## HTTP API

Контракт: [`contracts/openapi/trip-service.openapi.yaml`](contracts/openapi/trip-service.openapi.yaml).

| Метод | Маршрут | Назначение |
|---|---|---|
| `POST` | `/api/v1/trips` | Создать поездку; поддерживает `Idempotency-Key` |
| `GET` | `/api/v1/trips/{tripId}` | Получить поездку |
| `POST` | `/api/v1/trips/{tripId}/finish` | Завершить активную поездку |
| `GET` | `/health` | Проверить работу HTTP-сервера |
| `GET` | `/ready` | Проверить доступность PostgreSQL |

Ошибки возвращаются в формате `application/problem+json`: валидация — `400`, отсутствующая поездка — `404`, занятый водитель, повторное завершение и конфликт ключа — `409`. Внутренняя ошибка получает `500` без деталей ошибки в ответе.

## Архитектура

Путь запроса: HTTP-хендлер → usecase → интерфейсы репозиториев → PostgreSQL. Домен содержит модель поездки, правила переходов статусов и ошибки.

| Каталог | Ответственность |
|---|---|
| `cmd/service` | Чтение конфигурации, запуск приложения, обработка сигнала остановки |
| `internal/config` | Чтение и валидация переменных окружения |
| `internal/container` | Создание конкретных реализаций и сборка зависимостей |
| `internal/domain/trip` | Поездка, переходы статусов, доменные ошибки и ошибки валидации |
| `internal/model` | Данные для передачи между слоями |
| `internal/usecase/trip/{create,get,finish}` | Сценарии работы с поездками и границы транзакций |
| `internal/http` | Разбор запросов, преобразование моделей, ответы и middleware |
| `internal/http/domainerror` | Единый маппинг ошибок в HTTP-ответы |
| `internal/repository/postgres` | SQL-запросы и преобразование ошибок PostgreSQL |
| `internal/infra/postgres` | Пул, инструментирование запросов и менеджер транзакций |
| `internal/infra/telemetry` | Инициализация `slog`, OpenTelemetry и бизнес-метрик |
| `internal/infra/uuid` | Генерация идентификаторов |
| `internal/generated` | Код, сгенерированный по OpenAPI |

Интерфейсы объявлены у потребителей в `contract.go`: хендлер описывает нужный ему usecase, а usecase — методы репозиториев, менеджера транзакций и других зависимостей. Это позволяет заменять реализации и использовать моки. Зависимости передаются через конструкторы и связываются в `container.New`.

Домен не зависит от транспорта, драйвера БД и телеметрии. HTTP не обращается напрямую к репозиториям, usecase не использует `pgx` и не строит SQL. Правила импортов проверяет `depguard` в `.golangci.yml`.

### Транзакции и конкурентные запросы

`TxManager.Do` открывает транзакцию с изоляцией `READ COMMITTED` и сохраняет её в контексте. Репозитории через `transaction.Executor` выбирают транзакцию из контекста либо пул. Вложенный `Do` использует существующую транзакцию.

При успехе callback выполняется коммит. При ошибке или панике отложенный откат освобождает транзакцию; откат использует отдельный таймаут и контекст без отмены исходного запроса.

Создание поездки и начальной записи истории атомарно. Завершение поездки и запись перехода статуса также выполняются в одной транзакции.

Одна активная поездка на водителя обеспечивается частичным уникальным индексом:

```sql
CREATE UNIQUE INDEX trips_one_active_per_driver_idx
ON trips(driver_id)
WHERE status = 'active';
```

PostgreSQL `23505` преобразуется в `ErrDriverBusy` только при совпадении имени этого индекса.

При завершении usecase читает поездку через `SELECT ... FOR UPDATE`, вызывает доменный метод `Trip.Finish`, обновляет поездку и сохраняет историю. Домен разрешает завершение только из `StatusActive`. При двух конкурентных завершениях второй запрос после ожидания блокировки видит завершённую поездку и возвращает `409`, не перезаписывая `finished_at`.

### Идемпотентность создания

Ключи хранятся в таблице `trip_idempotency` вместе с SHA-256 от параметров запроса, идентификатором поездки и временем истечения. Срок действия задаётся `IDEMPOTENCY_TTL`.

Транзакционная advisory-блокировка по ключу сериализует запросы с одинаковым ключом. Повтор с теми же параметрами возвращает сохранённую поездку без создания новой; другие параметры с действующим ключом приводят к `409 idempotency_conflict`. Истёкший ключ удаляется при обращении к нему и может использоваться снова. Поездка, история и ключ сохраняются в общей транзакции.

## Проверки и генерация

| Команда | Действие |
|---|---|
| `make run` | Запустить `go run ./cmd/service` |
| `make generate` | Сгенерировать типы API и серверные интерфейсы |
| `make migrate` | Применить ожидающие миграции |
| `make migrate-down` | Откатить последнюю миграцию |
| `make migrate-status` | Показать состояние миграций |
| `make lint` | Запустить `golangci-lint` |
| `make test` | Запустить unit-тесты с `-race` и выводом покрытия |
| `make test-integration` | Запустить интеграционные тесты с реальным PostgreSQL |

```bash
make lint
make test
make test-integration
```

Unit-тесты проверяют доменные правила, usecase с `gomock`, маппинг ошибок и хендлеры через `httptest`. Работающая БД не требуется.

Интеграционные тесты отделены тегом `integration`. Нужен запущенный Docker: `testcontainers-go` поднимает отдельный PostgreSQL 17, применяет миграции и удаляет контейнер после теста. Окружение `tripgoctl` для этих тестов не требуется. Проверяются откат общей транзакции при ошибке записи истории, создание и чтение поездок, конкурентное создание поездок для одного водителя.

Моки генерируются через `go:generate` в `mocks_test.go` рядом с кодом и хранятся в репозитории. Для их обновления:

```bash
go generate ./internal/usecase/... ./internal/http/...
```

## Observability

Трейсы и метрики отправляются через OpenTelemetry SDK по OTLP в коллектор. Адрес и протокол задаются `OTEL_EXPORTER_OTLP_ENDPOINT` и `OTEL_EXPORTER_OTLP_PROTOCOL`. Отдельная ручка `/metrics` не требуется. Период экспорта метрик — 5 секунд.

В ресурс добавляются `service.name`, `service.version`, `service.namespace` и `deployment.environment`. Ссылки на Jaeger, Grafana и Prometheus выводит `tripgoctl connect`.

### Логи и трейсы

Логи пишутся через `log/slog` в JSON в stdout. Для записей внутри спана добавляются `trace_id` и `span_id`. HTTP middleware пишет одну запись `request handled` с методом, шаблоном маршрута, статусом и длительностью. `2xx` и `4xx` имеют уровень `Info`, `5xx` — `Error`. Тела запросов и ответов не логируются.

Создание и завершение поездки логируются после коммита; повтор по ключу не создаёт второе событие создания. Логируются запуск и остановка сервиса, ошибки сохранения и восстановленные паники со стеком.

Порядок middleware: `Recover → Tracing → Logging → Metrics → роутер`.

HTTP-спан называется по методу и шаблону маршрута, например `GET /api/v1/trips/{tripId}`. Внутри создаются спаны `trip.create`, `trip.get`, `trip.finish` и SQL-спаны через `otelpgx` на уровне пула. Контекст передаётся через `ctx`, входящий HTTP поддерживает W3C TraceContext. SQL-спаны содержат операцию и, если запрос обращается к таблице, её имя. Ошибки помечаются `codes.Error` и `error.type`; SQL-текст и параметры не записываются.

### Метрики

| Метрика OpenTelemetry | Тип | Атрибуты |
|---|---|---|
| `http.server.request.duration` | Histogram, секунды | `http.request.method`, `http.route`, `http.response.status_code` |
| `http.server.active_requests` | Up-down counter | `http.request.method`, `http.route` |
| `db.client.operation.duration` | Histogram, секунды | `db.operation.name`, `db.collection.name`, `error.type` при ошибке |
| `tripgo.trips.created` | Counter | — |
| `tripgo.trips.completed` | Counter | — |
| `tripgo.trips.active` | Observable gauge | — |

`http.route` содержит шаблон, без конкретного UUID. Бизнес-счётчики увеличиваются после коммита, создание по повторному ключу не учитывается второй раз. Gauge получает количество активных поездок из БД при сборе, поэтому учитывает поездки, созданные до запуска сервиса.

В Grafana Explore с источником Prometheus можно выполнять следующие запросы, каждый отдельно:

```promql
http_server_request_duration_seconds_count
```

```promql
db_client_operation_duration_seconds_count
```

```promql
http_server_active_requests
```

```promql
tripgo_trips_created_total
```

```promql
tripgo_trips_completed_total
```

```promql
tripgo_trips_active
```

Суффиксы `_count`, `_bucket` и `_sum` относятся к histogram, `_total` — к counter в представлении Prometheus.

Для проверки выполните создание, получение и завершение поездки, найдите в Jaeger трейс с HTTP- и SQL-спанами и проверьте его поиск по `trace_id` из лога. После экспорта проверьте метрики в Grafana. Ошибки отправки телеметрии логируются и не останавливают обслуживание запросов; это проверяется отдельно при временной недоступности коллектора. Скриншоты Jaeger и Grafana прикладываются к PR.

## Запуск в Docker

Поднимите окружение лабораторной работы 2, подготовьте `.env` и `.env.local` и примените миграции, как описано выше. Соберите образ из корня проекта:

```bash
docker build -f deploy/Dockerfile -t trip-service:lab2 .
```

Контейнер подключается к сети `kind` и обращается к NodePort-сервисам через имя узла кластера:

```bash
docker run --rm \
  --env-file .env \
  --env-file .env.local \
  --network kind \
  -p 8080:8080 \
  -e HTTP_ADDR=:8080 \
  -e 'DATABASE_URL=postgres://tripgo:tripgo@tripgo-local-control-plane:30203/tripgo?sslmode=disable' \
  -e OTEL_EXPORTER_OTLP_ENDPOINT=http://tripgo-local-control-plane:30206 \
  -e OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf \
  trip-service:lab2
```

`30203` и `30206` — NodePort PostgreSQL и OTLP HTTP для лабораторной работы 2 в стандартном кластере `tripgo-local`. Реквизиты БД должны совпадать с выданными окружением. В контейнере используется имя узла вместо `localhost`; HTTP API доступен на хосте по `http://localhost:8080`. Команда использует bridge-сеть Docker и не требует `network host`.
