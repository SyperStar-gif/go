package analytics

import (
	"testing"
	"time"
)

func TestGenerateDailyReport(t *testing.T) {
	svc := NewService()
	req := SimulationRequest{
		Sales24Months: []SaleRecord{
			{Timestamp: time.Date(2026, 10, 1, 13, 0, 0, 0, time.UTC), DishID: "soup_1", DishName: "Томатный суп", Category: "soups", Quantity: 10, Price: 300, Temperature: 12},
			{Timestamp: time.Date(2026, 10, 2, 13, 0, 0, 0, time.UTC), DishID: "soup_1", DishName: "Томатный суп", Category: "soups", Quantity: 9, Price: 300, Temperature: 14},
			{Timestamp: time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC), DishID: "drink_1", DishName: "Какао", Category: "hot_drinks", Quantity: 12, Price: 250, Temperature: 11},
			{Timestamp: time.Date(2026, 10, 2, 14, 0, 0, 0, time.UTC), DishID: "drink_1", DishName: "Какао", Category: "hot_drinks", Quantity: 11, Price: 250, Temperature: 13},
		},
		IngredientRequirements: []IngredientRequirement{
			{DishID: "soup_1", IngredientID: "beef", Ingredient: "Говядина", QuantityPerDish: 0.25, UOM: "кг"},
			{DishID: "drink_1", IngredientID: "milk", Ingredient: "Молоко", QuantityPerDish: 0.15, UOM: "л"},
		},
		Stock: []StockItem{
			{IngredientID: "beef", Ingredient: "Говядина", Quantity: 2, UOM: "кг"},
			{IngredientID: "milk", Ingredient: "Молоко", Quantity: 1, UOM: "л"},
		},
		Context:                  ExternalContext{Date: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), ForecastTemperature: 3, EventName: "Марафон", EventMultiplier: 1.2},
		HistoricalPlanFactPoints: []PlanFactPoint{{Label: "2026-10-20", Plan: 100, Fact: 115}, {Label: "2026-10-21", Plan: 130, Fact: 120}},
	}

	report, err := svc.GenerateDailyReport(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if report.TotalOrders <= 0 {
		t.Fatalf("expected total orders > 0")
	}
	if len(report.PurchasePlan) == 0 {
		t.Fatalf("expected purchase plan")
	}
	if len(report.Recommendations) == 0 {
		t.Fatalf("expected recommendations")
	}
	if report.PlanFact.WAPE <= 0 {
		t.Fatalf("expected non-zero WAPE")
	}
}

func TestGenerateDailyReportValidation(t *testing.T) {
	svc := NewService()
	_, err := svc.GenerateDailyReport(SimulationRequest{})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}
