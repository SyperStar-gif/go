# Subscription Service (Go)

Сервис предоставляет CRUDL API для подписок и расчет суммарной стоимости за период.

## Запуск

```bash
cp .env.example .env
docker compose up --build
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

## Бонус: самая банальная нейросеть

Добавлен минимальный пример в `cmd/nn-demo/main.go`: один нейрон (сигмоида), обучающийся на логическую операцию OR.

Запуск:

```bash
go run ./cmd/nn-demo
```
