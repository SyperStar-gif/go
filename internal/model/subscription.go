package model

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Delivery struct {
	ID                 uuid.UUID  `json:"id"`
	OrderNumber        string     `json:"order_number"`
	CustomerID         uuid.UUID  `json:"customer_id"`
	DestinationAddress string     `json:"destination_address"`
	Status             string     `json:"status"`
	Cost               int        `json:"cost"`
	DeliveryDate       time.Time  `json:"delivery_date"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

type DeliveryPayload struct {
	OrderNumber        string  `json:"order_number"`
	CustomerID         string  `json:"customer_id"`
	DestinationAddress string  `json:"destination_address"`
	Status             string  `json:"status"`
	Cost               int     `json:"cost"`
	DeliveryDate       string  `json:"delivery_date"`
	CompletedAt        *string `json:"completed_at,omitempty"`
}

var allowedStatuses = map[string]struct{}{
	"pending":    {},
	"in_transit": {},
	"delivered":  {},
	"canceled":   {},
}

func ParseDate(value string) (time.Time, error) {
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, errors.New("invalid date format, expected YYYY-MM-DD")
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

func (p DeliveryPayload) Validate() (Delivery, error) {
	if p.OrderNumber == "" {
		return Delivery{}, errors.New("order_number is required")
	}
	uid, err := uuid.Parse(p.CustomerID)
	if err != nil {
		return Delivery{}, errors.New("customer_id must be valid UUID")
	}
	if p.DestinationAddress == "" {
		return Delivery{}, errors.New("destination_address is required")
	}
	if _, ok := allowedStatuses[p.Status]; !ok {
		return Delivery{}, errors.New("status must be one of: pending, in_transit, delivered, canceled")
	}
	if p.Cost < 0 {
		return Delivery{}, errors.New("cost must be >= 0")
	}
	deliveryDate, err := ParseDate(p.DeliveryDate)
	if err != nil {
		return Delivery{}, err
	}

	var completedAt *time.Time
	if p.CompletedAt != nil && *p.CompletedAt != "" {
		parsedCompletedAt, err := ParseDate(*p.CompletedAt)
		if err != nil {
			return Delivery{}, err
		}
		if parsedCompletedAt.Before(deliveryDate) {
			return Delivery{}, errors.New("completed_at must be >= delivery_date")
		}
		completedAt = &parsedCompletedAt
	}

	return Delivery{
		OrderNumber:        p.OrderNumber,
		CustomerID:         uid,
		DestinationAddress: p.DestinationAddress,
		Status:             p.Status,
		Cost:               p.Cost,
		DeliveryDate:       deliveryDate,
		CompletedAt:        completedAt,
	}, nil
}
