package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"subscriptions/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

func scanSubscription(row pgx.Row) (model.Subscription, error) {
	var s model.Subscription
	err := row.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &s.StartDate, &s.EndDate, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SubscriptionRepository) Create(ctx context.Context, s model.Subscription) (model.Subscription, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (service_name, price, user_id, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at
	`, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)

	out, err := scanSubscription(row)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return out, nil
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id uuid.UUID) (model.Subscription, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, service_name, price, user_id, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE id = $1
	`, id)

	out, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return out, nil
}

func (r *SubscriptionRepository) Update(ctx context.Context, id uuid.UUID, s model.Subscription) (model.Subscription, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE subscriptions
		SET service_name = $2,
		    price = $3,
		    user_id = $4,
		    start_date = $5,
		    end_date = $6,
		    updated_at = now()
		WHERE id = $1
		RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at
	`, id, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)

	out, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return out, nil
}

func (r *SubscriptionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SubscriptionRepository) List(ctx context.Context, userID *uuid.UUID, serviceName string, limit, offset int) ([]model.Subscription, error) {
	query := `
		SELECT id, service_name, price, user_id, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE ($1::uuid IS NULL OR user_id = $1)
		  AND ($2::text IS NULL OR service_name ILIKE '%' || $2 || '%')
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, userID, nullableString(serviceName), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()

	out := make([]model.Subscription, 0)
	for rows.Next() {
		var s model.Subscription
		if err := rows.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &s.StartDate, &s.EndDate, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan list subscriptions: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows list subscriptions: %w", err)
	}
	return out, nil
}

func (r *SubscriptionRepository) TotalCost(ctx context.Context, periodStart, periodEnd time.Time, userID *uuid.UUID, serviceName string) (int64, error) {
	query := `
		WITH months AS (
			SELECT generate_series($1::date, $2::date, interval '1 month')::date AS month_start
		)
		SELECT COALESCE(SUM(s.price), 0)
		FROM months m
		JOIN subscriptions s
		  ON s.start_date <= m.month_start
		 AND (s.end_date IS NULL OR s.end_date >= m.month_start)
		WHERE ($3::uuid IS NULL OR s.user_id = $3)
		  AND ($4::text IS NULL OR s.service_name ILIKE '%' || $4 || '%')
	`

	var total int64
	if err := r.pool.QueryRow(ctx, query, periodStart, periodEnd, userID, nullableString(serviceName)).Scan(&total); err != nil {
		return 0, fmt.Errorf("total cost: %w", err)
	}
	return total, nil
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
