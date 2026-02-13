package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID          uuid.UUID  `json:"id"`
	ServiceName string     `json:"service_name"`
	Price       int        `json:"price"`
	UserID      uuid.UUID  `json:"user_id"`
	StartDate   time.Time  `json:"start_date"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type SubscriptionPayload struct {
	ServiceName string  `json:"service_name"`
	Price       int     `json:"price"`
	UserID      string  `json:"user_id"`
	StartDate   string  `json:"start_date"`
	EndDate     *string `json:"end_date,omitempty"`
}

func ParseMonthYear(value string) (time.Time, error) {
	t, err := time.Parse("01-2006", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month-year format %q, expected MM-YYYY", value)
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func FormatMonthYear(t time.Time) string {
	return t.Format("01-2006")
}

func (p SubscriptionPayload) Validate() (Subscription, error) {
	if p.ServiceName == "" {
		return Subscription{}, errors.New("service_name is required")
	}
	if p.Price < 0 {
		return Subscription{}, errors.New("price must be >= 0")
	}
	uid, err := uuid.Parse(p.UserID)
	if err != nil {
		return Subscription{}, errors.New("user_id must be valid UUID")
	}
	start, err := ParseMonthYear(p.StartDate)
	if err != nil {
		return Subscription{}, err
	}
	var end *time.Time
	if p.EndDate != nil && *p.EndDate != "" {
		parsedEnd, err := ParseMonthYear(*p.EndDate)
		if err != nil {
			return Subscription{}, err
		}
		if parsedEnd.Before(start) {
			return Subscription{}, errors.New("end_date must be >= start_date")
		}
		end = &parsedEnd
	}
	return Subscription{ServiceName: p.ServiceName, Price: p.Price, UserID: uid, StartDate: start, EndDate: end}, nil
}
