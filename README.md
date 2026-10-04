# Crypto Rates Service (GO-372)

Сервис периодически (по умолчанию раз в час) берёт курсы BTC и ETH с Bybit, пишет каждый снимок новой строкой в PostgreSQL и отдаёт историю через REST API и Telegram-бота. По этой истории считаются минимум и максимум за день и изменение цены за последний час.

## Архитектура

Проект построен по принципам **Clean Architecture**. Зависимости направлены **внутрь** — внешний слой (адаптеры) знает о внутреннем (domain), но не наоборот.

| Папка | Слой | Что делает |
| --- | --- | --- |
| `cmd/app` | composition root | Собирает зависимости: конфиг, логгер, пул БД, сервис, HTTP, бот. Бизнес-логики нет. |
| `internal/domain` | domain | Сущности (`Rate`), ошибки (`ErrNotFound`), константы (`SupportedCoins`). Не знает ни про базу, ни про HTTP. |
| `internal/interfaces` | ports (контракты) | Интерфейсы `Store`, `Exchange`, `Logger`. От них зависит `usecase`, их реализуют адаптеры. |
| `internal/usecase` | use case | Бизнес-логика: проценты, min/max, текст для бота, фоновый тикёр. |
| `internal/adapter/postgres` | adapter (склад) | Реализация `interfaces.Store` через `pgxpool`. |
| `internal/adapter/bybit` | adapter (биржа) | Реализация `interfaces.Exchange` через HTTP с поддержкой отмены по контексту. |
| `internal/config` | infrastructure | Загрузка `.env` + переменных окружения через `caarlos0/env`. |
| `internal/logger` | infrastructure | Настройка `uber-go/zap` с выводом в консоль и файл. |
| `internal/delivery/httpserver` | delivery | HTTP-роуты через `chi`. |
| `internal/delivery/telegram` | delivery | Telegram-бот. |

### Правила архитектуры

1. **Интерфейсы живут отдельно** — в `internal/interfaces/interfaces.go`. Пакет `usecase` импортирует только `interfaces` и `domain`, не зная о конкретных библиотеках (zap, pgx, http).
2. **Адаптеры реализуют интерфейсы** с компиляционной проверкой:

   ```go
   var _ interfaces.Store = (*PostgresRepo)(nil)
   var _ interfaces.Exchange = (*BybitClient)(nil)
   ```

   Если сигнатура в адаптере перестанет совпадать с контрактом — сборка упадёт.
3. **`main.go` — только Composition Root**: создание пула БД и его пинг происходит именно здесь, в `repo` передаётся уже готовый пул. Бизнес-логики и циклов тикера в `main` нет — это методы сервиса (`svc.Start`, `svc.Refresh`).
4. **Никакого хардкода**: URL биржи, интервал обновления, уровень логирования, адрес HTTP — всё читается из `.env` через `caarlos0/env`.
5. **Математика без костылей**: `changePercent` возвращает `*float64`, без флага `ok bool`. Отсутствие истории (`nil`) обрабатывается явно на уровне `usecase`.
6. **Go-стиль**: короткие ресиверы `(s *Service)`, `(r *PostgresRepo)`, `(c *BybitClient)`. Конструкторы говорящие: `NewService`, `NewPostgresRepo`, `NewBybitClient`.
7. **Один источник истины для монет**: список `domain.SupportedCoins` используется и в HTTP, и в боте, и в клиенте Bybit. Добавление новой монеты требует изменения только в одном месте.
8. **Граница суток в одном месте**: «сегодня» считается по Москве и в SQL, и в тексте для человека, поэтому нет расхождений на 3 часа.

При Ctrl+C процесс ждёт тикер, бота и HTTP, потом закрывает пул и лог. Если за сегодня нет строк, `MIN`/`MAX` из базы приходят как NULL — это не ноль и не паника, а доменная ошибка `ErrNotFound`, которая в JSON превращается в `null`.

## Технологии

- Go 1.25
- PostgreSQL 16, драйвер `jackc/pgx/v5`
- HTTP: `go-chi/chi/v5`
- Бот: `go-telegram-bot-api/telegram-bot-api/v5`
- Конфиг: `caarlos0/env/v11` + `joho/godotenv`
- Логи: `go.uber.org/zap`, текст в консоль и в `logs/app.log`
- Docker, Docker Compose
- CI: `go vet`, `go test -race -cover` (GitHub Actions)

Описание REST — в `openapi.yaml`.

## Запуск

### Через Docker Compose

Нужен Docker Desktop.

1. Скопируйте настройки:

   ```bash
   Copy-Item .env.example .env
   ```

2. В `.env` задайте пароль базы. Токен бота от @BotFather можно оставить пустым — тогда поднимется только REST. Файл `.env` в git не попадает.
3. Поднимаем стек:

   ```bash
   docker compose up --build
   ```

   Compose дождётся готовности Postgres, применит миграцию и откроет `http://localhost:8080`. История курсов лежит в томе `pgdata`. Вместе с историей:

   ```bash
   docker compose down -v
   ```

### Без Docker (локально)

1. Запустить базу (в отдельном терминале):

   ```bash
   docker compose up postgres
   ```

2. В корне проекта:

   ```bash
   go run ./cmd/app
   ```

Без `POSTGRES_URL` процесс сразу выходит: пароля в коде нет.

## Переменные окружения

| Переменная | Обязательна | Значение по умолчанию | Что делает |
| --- | --- | --- | --- |
| `POSTGRES_URL` | ✅ | — | Строка подключения к PostgreSQL (например, `postgres://user:pass@host:5432/db`) |
| `TELEGRAM_BOT_TOKEN` | ❌ | пусто | Токен от @BotFather; без него бот не стартует, но HTTP работает |
| `BYBIT_URL` | ❌ | `https://api.bybit.com/v5/market/tickers?category=spot&symbol=` | Базовый URL публичного API биржи |
| `HTTP_ADDR` | ❌ | `:8080` | Адрес HTTP-сервера (например, `:8080` или `127.0.0.1:9000`) |
| `UPDATE_INTERVAL` | ❌ | `1h` | Период опроса биржи (например, `15m`, `30s`, `2h`) |
| `LOG_LEVEL` | ❌ | `info` | Уровень логирования: `debug` / `info` / `warn` / `error` |
| `LOG_FILE` | ❌ | `logs/app.log` | Путь к файлу журнала (папка создаётся автоматически) |

Значения задаются либо в `.env`, либо переменными окружения процесса (Docker подставляет их из `docker-compose.yml`).

## HTTP

| Метод | Путь | Ответ |
| --- | --- | --- |
| GET | `/rates` | массив снимков BTC и ETH |
| GET | `/rates/{coin}` | одна монета, `404` если её нет в таблице |

Регистр в пути не важен: `/rates/btc` и `/rates/BTC` одно и то же.

```json
{
  "CoinSymbol": "BTC",
  "PriceUSD": 86000,
  "FetchedAt": "2026-09-28T18:00:00Z",
  "DayMin": 85000,
  "DayMax": 87000,
  "HourChangePercent": 1.18
}
```

Первый снимок пишется сразу при старте, дальше с периодом `UPDATE_INTERVAL` (по умолчанию — раз в час). Поля `DayMin`, `DayMax` и `HourChangePercent` будут `null`, если:
- за сегодня нет ни одной строки (`DayMin`, `DayMax`);
- нет цены старше часа (`HourChangePercent`).

`FetchedAt` в JSON — UTC.

## Telegram

| Команда | Что делает |
| --- | --- |
| `/start` | список команд |
| `/rates` | курсы BTC и ETH |
| `/rates BTC` | одна монета |
| `/rates_btc`, `/rates_eth` | только биткоин или только эфир |
| `/start_auto N` | присылать курсы каждые N минут |
| `/start_auto_10` | то же с периодом 10 минут |
| `/stop_auto` | выключить авторассылку |

Telegram подсвечивает команду до подчёркивания, поэтому `/start-auto 15` тоже принимается. После перезапуска сервиса рассылку нужно включить снова: она живёт в памяти процесса.

## Тесты

По умолчанию без живой базы и без интернета. Вместо Postgres и Bybit стоят заглушки, HTTP проверяется через `httptest`.

```bash
go test ./...
go vet ./...
go test -race -cover ./...
```

Интеграционный тест склада (NULL от живого Postgres) пропускается без `POSTGRES_TEST_URL`:

```bash
# Bash
POSTGRES_TEST_URL='postgres://krypto:change-me@localhost:5432/krypto' go test ./internal/adapter/postgres/...

# PowerShell
$env:POSTGRES_TEST_URL = "postgres://krypto:change-me@localhost:5432/krypto"
go test -count=1 -race -cover ./internal/adapter/postgres/...
```

Покрытие построчно:

```bash
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```