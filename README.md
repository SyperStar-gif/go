# Subscription Service (Go)

Сервис предоставляет CRUDL API для подписок и расчет суммарной стоимости за период.

## Варианты хранилища

1. `postgres` (по умолчанию)
2. `local` — локальная JSON-база прямо в проекте (`localdb/subscriptions.json`)

## Запуск (PostgreSQL через docker compose)

```bash
cp .env.example .env
docker compose up --build
```

## Запуск с локальной БД (без PostgreSQL)

```bash
cp .env.example .env
# в .env выставить STORAGE_MODE=local
# (опционально) LOCAL_DB_PATH=localdb/subscriptions.json
go run ./cmd/server
```

## API

- Swagger: `GET /swagger.yaml`
- Health: `GET /health`
- CRUDL: `/api/v1/subscriptions/`
- Aggregation: `GET /api/v1/subscriptions/total?period_start=01-2025&period_end=03-2025&user_id=<uuid>&service_name=Yandex`

## Пример create

```json
{
  "service_name": "Yandex Plus",
  "price": 400,
  "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
  "start_date": "07-2025"
}
```
