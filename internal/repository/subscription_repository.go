package repository

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"subscriptions/internal/model"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type SubscriptionRepository struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
}

func NewSubscriptionRepository(pool *pgxpool.Pool, logger *slog.Logger) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool, logger: logger}
}

func scanSubscription(row pgx.Row) (model.Subscription, error) {
	var s model.Subscription
	err := row.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &s.StartDate, &s.EndDate, &s.CreatedAt, &s.UpdatedAt)
	return s, err
}

func (r *SubscriptionRepository) Create(ctx context.Context, s model.Subscription) (model.Subscription, error) {
	r.logger.Info("creating subscription", "service_name", s.ServiceName, "user_id", s.UserID)
	row := r.pool.QueryRow(ctx, `
		INSERT INTO subscriptions (service_name, price, user_id, start_date, end_date)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at
	`, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)
	created, err := scanSubscription(row)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	return created, nil
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id uuid.UUID) (model.Subscription, error) {
	row := r.pool.QueryRow(ctx, `SELECT id, service_name, price, user_id, start_date, end_date, created_at, updated_at FROM subscriptions WHERE id=$1`, id)
	s, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	return s, nil
}

func (r *SubscriptionRepository) Update(ctx context.Context, id uuid.UUID, s model.Subscription) (model.Subscription, error) {
	r.logger.Info("updating subscription", "id", id)
	row := r.pool.QueryRow(ctx, `
		UPDATE subscriptions SET service_name=$2, price=$3, user_id=$4, start_date=$5, end_date=$6, updated_at=now()
		WHERE id=$1
		RETURNING id, service_name, price, user_id, start_date, end_date, created_at, updated_at
	`, id, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)
	updated, err := scanSubscription(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return updated, nil
}

func (r *SubscriptionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	r.logger.Info("deleting subscription", "id", id)
	cmd, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SubscriptionRepository) List(ctx context.Context, userID, serviceName string, limit, offset int) ([]model.Subscription, error) {
	args := []any{}
	conds := []string{"1=1"}
	if userID != "" {
		args = append(args, userID)
		conds = append(conds, fmt.Sprintf("user_id = $%d", len(args)))
	}
	if serviceName != "" {
		args = append(args, serviceName)
		conds = append(conds, fmt.Sprintf("service_name ILIKE $%d", len(args)))
	}
	args = append(args, limit, offset)
	query := fmt.Sprintf(`
		SELECT id, service_name, price, user_id, start_date, end_date, created_at, updated_at
		FROM subscriptions
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(conds, " AND "), len(args)-1, len(args))
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	defer rows.Close()
	var out []model.Subscription
	for rows.Next() {
		var s model.Subscription
		if err := rows.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &s.StartDate, &s.EndDate, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan list subscription: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SubscriptionRepository) TotalCost(ctx context.Context, periodStart, periodEnd time.Time, userID, serviceName string) (int64, error) {
	args := []any{periodStart, periodEnd}
	conds := []string{"s.start_date <= m.month_start", "(s.end_date IS NULL OR s.end_date >= m.month_start)"}
	if userID != "" {
		args = append(args, userID)
		conds = append(conds, fmt.Sprintf("s.user_id = $%d::uuid", len(args)))
	}
	if serviceName != "" {
		args = append(args, serviceName)
		conds = append(conds, fmt.Sprintf("s.service_name ILIKE $%d", len(args)))
	}
	query := fmt.Sprintf(`
		WITH months AS (
			SELECT generate_series($1::date, $2::date, interval '1 month')::date AS month_start
		)
		SELECT COALESCE(SUM(s.price), 0)
		FROM months m
		JOIN subscriptions s ON %s
	`, strings.Join(conds, " AND "))

	var total int64
	if err := r.pool.QueryRow(ctx, query, args...).Scan(&total); err != nil {
		return 0, fmt.Errorf("total cost: %w", err)
	}
	return total, nil
}
