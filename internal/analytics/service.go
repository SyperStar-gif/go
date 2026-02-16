package analytics

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"
)

const defaultBufferPercent = 0.10

type SaleRecord struct {
	Timestamp   time.Time `json:"timestamp"`
	DishID      string    `json:"dish_id"`
	DishName    string    `json:"dish_name"`
	Category    string    `json:"category"`
	Quantity    int       `json:"quantity"`
	Price       float64   `json:"price"`
	Temperature float64   `json:"temperature"`
	IsHoliday   bool      `json:"is_holiday"`
	EventFactor float64   `json:"event_factor"`
}

type IngredientRequirement struct {
	DishID          string  `json:"dish_id"`
	IngredientID    string  `json:"ingredient_id"`
	Ingredient      string  `json:"ingredient"`
	QuantityPerDish float64 `json:"quantity_per_dish"`
	UOM             string  `json:"uom"`
}

type StockItem struct {
	IngredientID string  `json:"ingredient_id"`
	Ingredient   string  `json:"ingredient"`
	Quantity     float64 `json:"quantity"`
	UOM          string  `json:"uom"`
}

type ExternalContext struct {
	Date                time.Time `json:"date"`
	ForecastTemperature float64   `json:"forecast_temperature"`
	IsHoliday           bool      `json:"is_holiday"`
	EventName           string    `json:"event_name,omitempty"`
	EventMultiplier     float64   `json:"event_multiplier"`
}

type ForecastItem struct {
	DishID            string  `json:"dish_id"`
	DishName          string  `json:"dish_name"`
	Category          string  `json:"category"`
	PredictedQuantity int     `json:"predicted_quantity"`
	PredictedRevenue  float64 `json:"predicted_revenue"`
}

type CategoryForecast struct {
	Category          string  `json:"category"`
	PredictedQuantity int     `json:"predicted_quantity"`
	PredictedRevenue  float64 `json:"predicted_revenue"`
}

type HourForecast struct {
	Hour            int `json:"hour"`
	PredictedOrders int `json:"predicted_orders"`
}

type PurchaseItem struct {
	IngredientID string  `json:"ingredient_id"`
	Ingredient   string  `json:"ingredient"`
	Need         float64 `json:"need"`
	Stock        float64 `json:"stock"`
	Purchase     float64 `json:"purchase"`
	UOM          string  `json:"uom"`
}

type PlanFactMetrics struct {
	MAPE float64 `json:"mape"`
	WAPE float64 `json:"wape"`
	Bias float64 `json:"bias"`
	RMSE float64 `json:"rmse"`
}

type PlanFactPoint struct {
	Label string  `json:"label"`
	Plan  float64 `json:"plan"`
	Fact  float64 `json:"fact"`
}

type BusinessRecommendation struct {
	Title  string `json:"title"`
	Reason string `json:"reason"`
	Action string `json:"action"`
}

type DailyForecastReport struct {
	Date            string                   `json:"date"`
	TotalRevenue    float64                  `json:"total_revenue"`
	TotalOrders     int                      `json:"total_orders"`
	ByDish          []ForecastItem           `json:"by_dish"`
	ByCategory      []CategoryForecast       `json:"by_category"`
	HourlyLoad      []HourForecast           `json:"hourly_load"`
	PurchasePlan    []PurchaseItem           `json:"purchase_plan"`
	PlanFact        PlanFactMetrics          `json:"plan_fact"`
	Recommendations []BusinessRecommendation `json:"recommendations"`
}

type SimulationRequest struct {
	Sales24Months            []SaleRecord            `json:"sales_24_months"`
	IngredientRequirements   []IngredientRequirement `json:"ingredient_requirements"`
	Stock                    []StockItem             `json:"stock"`
	Context                  ExternalContext         `json:"context"`
	BufferPercent            *float64                `json:"buffer_percent,omitempty"`
	HistoricalPlanFactPoints []PlanFactPoint         `json:"historical_plan_fact_points,omitempty"`
}

type Service struct{}

func NewService() *Service { return &Service{} }

func (s *Service) GenerateDailyReport(req SimulationRequest) (DailyForecastReport, error) {
	if len(req.Sales24Months) == 0 {
		return DailyForecastReport{}, errors.New("sales_24_months is required")
	}
	if req.Context.Date.IsZero() {
		return DailyForecastReport{}, errors.New("context.date is required")
	}
	bufferPercent := defaultBufferPercent
	if req.BufferPercent != nil {
		bufferPercent = *req.BufferPercent
	}
	if bufferPercent < 0 || bufferPercent > 1 {
		return DailyForecastReport{}, errors.New("buffer_percent must be between 0 and 1")
	}

	forecast := buildDishForecast(req.Sales24Months, req.Context)
	category := aggregateCategory(forecast)
	hourly := buildHourlyForecast(req.Sales24Months, req.Context)
	purchase := buildPurchasePlan(forecast, req.IngredientRequirements, req.Stock, bufferPercent)
	pf := calculatePlanFactMetrics(req.HistoricalPlanFactPoints)
	reco := buildRecommendations(forecast, category, purchase, req.Context)

	totalOrders := 0
	totalRevenue := 0.0
	for _, item := range forecast {
		totalOrders += item.PredictedQuantity
		totalRevenue += item.PredictedRevenue
	}

	return DailyForecastReport{
		Date:            req.Context.Date.Format("2006-01-02"),
		TotalRevenue:    round2(totalRevenue),
		TotalOrders:     totalOrders,
		ByDish:          forecast,
		ByCategory:      category,
		HourlyLoad:      hourly,
		PurchasePlan:    purchase,
		PlanFact:        pf,
		Recommendations: reco,
	}, nil
}

func buildDishForecast(sales []SaleRecord, context ExternalContext) []ForecastItem {
	type dishStat struct {
		DishID       string
		DishName     string
		Category     string
		Qty          int
		Revenue      float64
		Days         map[string]struct{}
		Temp         float64
		TempCount    int
		HolidayQty   int
		HolidayCount int
		RegularQty   int
		RegularCount int
	}
	byDish := map[string]*dishStat{}
	for _, s := range sales {
		key := s.DishID
		if key == "" {
			key = s.DishName
		}
		if key == "" || s.Quantity <= 0 || s.Price < 0 {
			continue
		}
		st, ok := byDish[key]
		if !ok {
			st = &dishStat{DishID: s.DishID, DishName: s.DishName, Category: s.Category, Days: map[string]struct{}{}}
			byDish[key] = st
		}
		st.Qty += s.Quantity
		st.Revenue += float64(s.Quantity) * s.Price
		st.Days[s.Timestamp.Format("2006-01-02")] = struct{}{}
		st.Temp += s.Temperature
		st.TempCount++
		if s.IsHoliday {
			st.HolidayQty += s.Quantity
			st.HolidayCount++
		} else {
			st.RegularQty += s.Quantity
			st.RegularCount++
		}
	}

	items := make([]ForecastItem, 0, len(byDish))
	for _, st := range byDish {
		days := len(st.Days)
		if days == 0 {
			continue
		}
		base := float64(st.Qty) / float64(days)
		avgTemp := 0.0
		if st.TempCount > 0 {
			avgTemp = st.Temp / float64(st.TempCount)
		}

		tempShift := context.ForecastTemperature - avgTemp
		tempCoef := 1.0
		if st.Category == "soups" || st.Category == "hot_drinks" {
			tempCoef = 1.0 + clamp((-tempShift)*0.03, -0.25, 0.8)
		} else if st.Category == "salads" || st.Category == "cold_drinks" {
			tempCoef = 1.0 + clamp(tempShift*0.02, -0.35, 0.35)
		}

		holidayCoef := 1.0
		if context.IsHoliday {
			holidayCoef += 0.15
			if st.HolidayCount > 0 && st.RegularCount > 0 {
				holidayAvg := float64(st.HolidayQty) / float64(st.HolidayCount)
				regularAvg := float64(st.RegularQty) / float64(st.RegularCount)
				if regularAvg > 0 {
					holidayCoef *= clamp(holidayAvg/regularAvg, 0.7, 1.6)
				}
			}
		}

		eventCoef := context.EventMultiplier
		if eventCoef == 0 {
			eventCoef = 1
		}

		predicted := int(math.Round(base * tempCoef * holidayCoef * eventCoef))
		if predicted < 0 {
			predicted = 0
		}

		avgPrice := st.Revenue / float64(st.Qty)
		items = append(items, ForecastItem{
			DishID:            st.DishID,
			DishName:          st.DishName,
			Category:          st.Category,
			PredictedQuantity: predicted,
			PredictedRevenue:  round2(float64(predicted) * avgPrice),
		})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].PredictedRevenue > items[j].PredictedRevenue })
	return items
}

func aggregateCategory(items []ForecastItem) []CategoryForecast {
	by := map[string]CategoryForecast{}
	for _, item := range items {
		v := by[item.Category]
		v.Category = item.Category
		v.PredictedQuantity += item.PredictedQuantity
		v.PredictedRevenue = round2(v.PredictedRevenue + item.PredictedRevenue)
		by[item.Category] = v
	}
	result := make([]CategoryForecast, 0, len(by))
	for _, v := range by {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].PredictedRevenue > result[j].PredictedRevenue })
	return result
}

func buildHourlyForecast(sales []SaleRecord, context ExternalContext) []HourForecast {
	byHour := make(map[int]int)
	dayCount := map[string]struct{}{}
	for _, s := range sales {
		if s.Quantity <= 0 {
			continue
		}
		dayCount[s.Timestamp.Format("2006-01-02")] = struct{}{}
		byHour[s.Timestamp.Hour()] += s.Quantity
	}
	days := len(dayCount)
	if days == 0 {
		return nil
	}
	mult := context.EventMultiplier
	if mult == 0 {
		mult = 1
	}
	out := make([]HourForecast, 0, 24)
	for hour := 0; hour < 24; hour++ {
		avg := float64(byHour[hour]) / float64(days)
		value := int(math.Round(avg * mult))
		if value < 0 {
			value = 0
		}
		if value > 0 {
			out = append(out, HourForecast{Hour: hour, PredictedOrders: value})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Hour < out[j].Hour })
	return out
}

func buildPurchasePlan(forecast []ForecastItem, requirements []IngredientRequirement, stock []StockItem, bufferPercent float64) []PurchaseItem {
	dishQty := map[string]int{}
	for _, f := range forecast {
		dishQty[f.DishID] = f.PredictedQuantity
	}
	if len(dishQty) == 0 {
		return nil
	}
	need := map[string]PurchaseItem{}
	for _, req := range requirements {
		qty := dishQty[req.DishID]
		if qty <= 0 {
			continue
		}
		v := need[req.IngredientID]
		v.IngredientID = req.IngredientID
		v.Ingredient = req.Ingredient
		v.UOM = req.UOM
		v.Need += float64(qty) * req.QuantityPerDish
		need[req.IngredientID] = v
	}

	stockByID := map[string]StockItem{}
	for _, s := range stock {
		stockByID[s.IngredientID] = s
	}

	out := make([]PurchaseItem, 0, len(need))
	for id, v := range need {
		st := stockByID[id]
		v.Stock = st.Quantity
		net := (v.Need - v.Stock) * (1 + bufferPercent)
		if net < 0 {
			net = 0
		}
		v.Need = round2(v.Need)
		v.Stock = round2(v.Stock)
		v.Purchase = round2(net)
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Purchase > out[j].Purchase })
	return out
}

func calculatePlanFactMetrics(points []PlanFactPoint) PlanFactMetrics {
	if len(points) == 0 {
		return PlanFactMetrics{}
	}
	var (
		sumAPE     float64
		sumAbsErr  float64
		sumFact    float64
		sumErr     float64
		sumSquared float64
		validAPE   int
	)
	for _, p := range points {
		err := p.Fact - p.Plan
		absErr := math.Abs(err)
		if p.Fact != 0 {
			sumAPE += absErr / math.Abs(p.Fact)
			validAPE++
		}
		sumAbsErr += absErr
		sumFact += math.Abs(p.Fact)
		sumErr += err
		sumSquared += err * err
	}
	mape := 0.0
	if validAPE > 0 {
		mape = sumAPE / float64(validAPE) * 100
	}
	wape := 0.0
	if sumFact > 0 {
		wape = (sumAbsErr / sumFact) * 100
	}
	bias := sumErr / float64(len(points))
	rmse := math.Sqrt(sumSquared / float64(len(points)))

	return PlanFactMetrics{MAPE: round2(mape), WAPE: round2(wape), Bias: round2(bias), RMSE: round2(rmse)}
}

func buildRecommendations(forecast []ForecastItem, categories []CategoryForecast, purchases []PurchaseItem, context ExternalContext) []BusinessRecommendation {
	out := make([]BusinessRecommendation, 0, 4)
	if len(categories) > 0 {
		top := categories[0]
		out = append(out, BusinessRecommendation{
			Title:  fmt.Sprintf("Фокус на категории %s", top.Category),
			Reason: fmt.Sprintf("Категория дает максимальную прогнозную выручку: %.2f", top.PredictedRevenue),
			Action: "Проверьте заготовки и персонал на этой линии перед пиком.",
		})
	}
	if context.ForecastTemperature <= 5 {
		out = append(out, BusinessRecommendation{
			Title:  "Ожидается похолодание",
			Reason: fmt.Sprintf("Прогноз температуры %.1f°C", context.ForecastTemperature),
			Action: "Увеличьте заготовки горячих блюд и согревающих напитков.",
		})
	}
	if context.EventName != "" && context.EventMultiplier > 1 {
		out = append(out, BusinessRecommendation{
			Title:  "Локальное событие рядом с рестораном",
			Reason: fmt.Sprintf("Событие '%s' повышает спрос, multiplier=%.2f", context.EventName, context.EventMultiplier),
			Action: "Подготовьте временное предложение на вынос и ускорьте выдачу.",
		})
	}
	for _, p := range purchases {
		if p.Purchase > 0 {
			out = append(out, BusinessRecommendation{
				Title:  fmt.Sprintf("Нужна дозакупка: %s", p.Ingredient),
				Reason: fmt.Sprintf("Потребность %.2f %s, остаток %.2f %s", p.Need, p.UOM, p.Stock, p.UOM),
				Action: fmt.Sprintf("Закупить минимум %.2f %s", p.Purchase, p.UOM),
			})
			break
		}
	}
	if len(out) == 0 && len(forecast) > 0 {
		out = append(out, BusinessRecommendation{
			Title:  "Спрос стабилен",
			Reason: "По данным прогноза нет критичных рисков.",
			Action: "Работайте в штатном режиме и отслеживайте план-факт в течение дня.",
		})
	}
	return out
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
