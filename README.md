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
- Analytics simulation (restaurant MVP): `POST /api/v1/analytics/simulate`

## Пример create

```json
{
  "service_name": "Yandex Plus",
  "price": 400,
  "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
  "start_date": "07-2025"
}
```

## Дополнительная документация

- Пример расширенного ТЗ для аналитической системы ресторана: `docs/restaurant-demand-analytics-tz.md`

## Пример analytics simulate

```json
{
  "sales_24_months": [
    {
      "timestamp": "2026-10-01T13:00:00Z",
      "dish_id": "soup_1",
      "dish_name": "Томатный суп",
      "category": "soups",
      "quantity": 10,
      "price": 300,
      "temperature": 12,
      "is_holiday": false
    }
  ],
  "ingredient_requirements": [
    {
      "dish_id": "soup_1",
      "ingredient_id": "beef",
      "ingredient": "Говядина",
      "quantity_per_dish": 0.25,
      "uom": "кг"
    }
  ],
  "stock": [
    {
      "ingredient_id": "beef",
      "ingredient": "Говядина",
      "quantity": 2,
      "uom": "кг"
    }
  ],
  "context": {
    "date": "2026-10-26T00:00:00Z",
    "forecast_temperature": 3,
    "is_holiday": false,
    "event_name": "Городской марафон",
    "event_multiplier": 1.2
  },
  "buffer_percent": 0.1,
  "historical_plan_fact_points": [
    {
      "label": "2026-10-20",
      "plan": 100,
      "fact": 115
    }
  ]
}
```
