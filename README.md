# Crypto Rates Service (GO-372)

Сервис раз в час берёт курсы BTC и ETH с Bybit, пишет каждый снимок новой строкой в PostgreSQL
и отдаёт историю через REST API и Telegram-бота. По этой истории считаются минимум и максимум
за день и изменение цены за последний час.

## Архитектура

Папки названы по ролям. По смыслу это тот же разбор по смыслу так же (domain / use case / adapter).

| Папка | Слои | Что делает |
| --- | --- | --- |
| `model` | domain | Бланк `Rate`, список монет `SupportedCoins`, ошибка `ErrNotFound`. Не знает ни про базу, ни про HTTP. |
| `service` | use case | Проценты, min/max, текст для бота. Здесь объявлены интерфейсы `Store` и `Exchange`. |
| `repository`, `client` | adapter | Postgres через `pgxpool` и HTTP к Bybit. Кухня про них не знает. |
| `httpserver`, `telegram` | delivery | Браузер и бот. SQL тут нет. |
| `main.go` | composition root | Собирает всех вместе: `service.New(store, client.New())`. Здесь же остановка процесса. |

Зависимости идут к центру. Поэтому замена CoinGecko (когда биржа отказывать стала) на Bybit затронула только `client`.

При Ctrl+C процесс ждёт тикер, бота и HTTP, потом закрывает пул и лог. Если за сегодня нет строк,
`MIN`/`MAX` из базы приходят как NULL — это не ноль и не паника.

## Технологии

- Go 1.25
- PostgreSQL 16, драйвер `jackc/pgx/v5`
- HTTP: `go-chi/chi/v5`
- Бот: `go-telegram-bot-api/telegram-bot-api/v5`
- Логи: `log/slog`, текст в `logs/app.log` и в консоль
- Docker, Docker Compose
- GitLab CI: `go vet`, `go test -race -cover`, сборка образа

Описание REST — в `openapi.yaml`.

## Запуск

Нужен Docker Desktop.

1. Скопируйте настройки:

   ```bash
   Copy-Item .env.example .env
   ```

2. В `.env` задайте пароль базы. Токен бота от @BotFather можно оставить пустым — тогда
   поднимется только REST. Файл `.env` в git не попадает.

3. Поднимаем стек:

   ```bash
   docker compose up --build
   ```

Compose дождётся готовности Postgres, применит миграцию и откроет `http://localhost:8080`.
История курсов лежит в томе `pgdata`. 
Вместе с историей: `docker compose down -v`.

Проверка:

```bash
curl http://localhost:8080/rates
curl http://localhost:8080/rates/BTC
```

Первый снимок пишется сразу при старте, дальше каждый час. Процент за час появится,
когда в базе будет строка старше часа.

Без Docker:

```bash
export POSTGRES_URL='postgres://krypto:change-me@localhost:5432/krypto'
export TELEGRAM_BOT_TOKEN='...'
go run .
```

Без `POSTGRES_URL` процесс сразу выходит: пароля в коде нет.

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

`HourChangePercent` будет `null`, пока нет цены старше часа. `FetchedAt` в JSON — UTC.

## Telegram

| Команда | Что делает |
| --- | --- |
| `/start` | список команд |
| `/rates` | курсы BTC и ETH |
| `/rates BTC` | одна монета |
| `/rates_btc`, `/rates_eth` | только биткоин или только эфир |
| `/start_auto 15` | присылать курсы каждые 15 минут |
| `/start_auto_10` | то же с периодом 10 минут |
| `/stop_auto` | выключить авторассылку |

Telegram подсвечивает команду до подчёркивания, поэтому `/start-auto 15` тоже принимается.
После перезапуска сервиса рассылку нужно включить снова: она живёт в памяти процесса.

## Тесты

По умолчанию без живой базы и без интернета. Вместо Postgres и Bybit стоят заглушки,
HTTP проверяется через `httptest`.

```bash
go test ./...
go vet ./...
go test -race -cover ./...
```

Интеграционный тест склада (NULL от живого Postgres) пропускается без `POSTGRES_TEST_URL`:

```bash
POSTGRES_TEST_URL='postgres://krypto:change-me@localhost:5432/krypto' go test ./repository/...
```

Покрытие построчно:

```bash
go test -coverprofile=cover.out ./...
go tool cover -html=cover.out
```

## CI/CD

В `.gitlab-ci.yml` две стадии. Сначала `test`, потом `publish` — только если тесты зелёные.
Образ уходит в GitLab Container Registry с тегом коммита. Тег `latest` ставится только
на ветке по умолчанию. Логин в registry — встроенные переменные GitLab, свои секреты
для этого добавлять не нужно.

## Чего нет

Авторассылка бота не переживает перезапуск. Список монет общий в `model.SupportedCoins`,
но пары Bybit (`BTCUSDT`, `ETHUSDT`) всё ещё прописаны в клиенте. Журнал пишет Info и Error,
уровень при старте не переключается.
