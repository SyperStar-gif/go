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

type DeliveryRepository struct {
	pool *pgxpool.Pool
}

func NewDeliveryRepository(pool *pgxpool.Pool) *DeliveryRepository {
	return &DeliveryRepository{pool: pool}
}

func scanDelivery(row pgx.Row) (model.Delivery, error) {
	var d model.Delivery
	err := row.Scan(&d.ID, &d.OrderNumber, &d.CustomerID, &d.DestinationAddress, &d.Status, &d.Cost, &d.DeliveryDate, &d.CompletedAt, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (r *DeliveryRepository) Create(ctx context.Context, d model.Delivery) (model.Delivery, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO deliveries (order_number, customer_id, destination_address, status, cost, delivery_date, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, order_number, customer_id, destination_address, status, cost, delivery_date, completed_at, created_at, updated_at
	`, d.OrderNumber, d.CustomerID, d.DestinationAddress, d.Status, d.Cost, d.DeliveryDate, d.CompletedAt)

	out, err := scanDelivery(row)
	if err != nil {
		return model.Delivery{}, fmt.Errorf("create delivery: %w", err)
	}
	return out, nil
}

func (r *DeliveryRepository) GetByID(ctx context.Context, id uuid.UUID) (model.Delivery, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, order_number, customer_id, destination_address, status, cost, delivery_date, completed_at, created_at, updated_at
		FROM deliveries
		WHERE id = $1
	`, id)

	out, err := scanDelivery(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Delivery{}, ErrNotFound
		}
		return model.Delivery{}, fmt.Errorf("get delivery: %w", err)
	}
	return out, nil
}

func (r *DeliveryRepository) Update(ctx context.Context, id uuid.UUID, d model.Delivery) (model.Delivery, error) {
	row := r.pool.QueryRow(ctx, `
		UPDATE deliveries
		SET order_number = $2,
		    customer_id = $3,
		    destination_address = $4,
		    status = $5,
		    cost = $6,
		    delivery_date = $7,
		    completed_at = $8,
		    updated_at = now()
		WHERE id = $1
		RETURNING id, order_number, customer_id, destination_address, status, cost, delivery_date, completed_at, created_at, updated_at
	`, id, d.OrderNumber, d.CustomerID, d.DestinationAddress, d.Status, d.Cost, d.DeliveryDate, d.CompletedAt)

	out, err := scanDelivery(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Delivery{}, ErrNotFound
		}
		return model.Delivery{}, fmt.Errorf("update delivery: %w", err)
	}
	return out, nil
}

func (r *DeliveryRepository) Delete(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM deliveries WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete delivery: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *DeliveryRepository) List(ctx context.Context, customerID *uuid.UUID, status string, limit, offset int) ([]model.Delivery, error) {
	query := `
		SELECT id, order_number, customer_id, destination_address, status, cost, delivery_date, completed_at, created_at, updated_at
		FROM deliveries
		WHERE ($1::uuid IS NULL OR customer_id = $1)
		  AND ($2::text IS NULL OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`

	rows, err := r.pool.Query(ctx, query, customerID, nullableString(status), limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	out := make([]model.Delivery, 0)
	for rows.Next() {
		var d model.Delivery
		if err := rows.Scan(&d.ID, &d.OrderNumber, &d.CustomerID, &d.DestinationAddress, &d.Status, &d.Cost, &d.DeliveryDate, &d.CompletedAt, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan list deliveries: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows list deliveries: %w", err)
	}
	return out, nil
}

func (r *DeliveryRepository) TotalCost(ctx context.Context, fromDate, toDate time.Time, customerID *uuid.UUID, status string) (int64, error) {
	query := `
		SELECT COALESCE(SUM(cost), 0)
		FROM deliveries
		WHERE delivery_date >= $1
		  AND delivery_date <= $2
		  AND ($3::uuid IS NULL OR customer_id = $3)
		  AND ($4::text IS NULL OR status = $4)
	`

	var total int64
	if err := r.pool.QueryRow(ctx, query, fromDate, toDate, customerID, nullableString(status)).Scan(&total); err != nil {
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
