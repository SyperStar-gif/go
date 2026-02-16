# Delivery Service (Go)

Сервис предоставляет CRUDL API для доставок и расчет суммарной стоимости за выбранный период.

## Запуск

```bash
docker compose up --build
```

## API

- Swagger: `GET /swagger.yaml`
- Health: `GET /health`
- CRUDL: `/api/v1/deliveries/`
- Aggregation: `GET /api/v1/deliveries/total?date_from=2025-01-01&date_to=2025-01-31&customer_id=<uuid>&status=delivered`

## Пример create

```json
{
  "order_number": "ORD-2025-001",
  "customer_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
  "destination_address": "Moscow, Tverskaya 1",
  "status": "pending",
  "cost": 900,
  "delivery_date": "2025-07-11"
}
```
