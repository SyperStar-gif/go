package repository

import (
	"context"
	"time"

	"subscriptions/internal/model"

	"github.com/google/uuid"
)

type SubscriptionStore interface {
	Create(ctx context.Context, s model.Subscription) (model.Subscription, error)
	GetByID(ctx context.Context, id uuid.UUID) (model.Subscription, error)
	Update(ctx context.Context, id uuid.UUID, s model.Subscription) (model.Subscription, error)
	Delete(ctx context.Context, id uuid.UUID) error
	List(ctx context.Context, userID *uuid.UUID, serviceName string, limit, offset int) ([]model.Subscription, error)
	TotalCost(ctx context.Context, periodStart, periodEnd time.Time, userID *uuid.UUID, serviceName string) (int64, error)
}
