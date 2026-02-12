#!/usr/bin/env bash
set -euo pipefail

# =========
# Effective Mobile (Junior Go) - Subscriptions service (Gin + Postgres + goose + swagger + docker-compose)
# One-shot bootstrap: creates a working repo you can "docker compose up --build"
# =========

PROJECT_DIR="subs-service"
mkdir -p "$PROJECT_DIR"
cd "$PROJECT_DIR"

mkdir -p cmd/app internal/{config,db,http,model,repo,util} migrations docs

cat > go.mod <<'EOF'
module subs-service

go 1.22

require (
	github.com/gin-gonic/gin v1.10.0
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.7.1
	github.com/swaggo/files v1.0.1
	github.com/swaggo/gin-swagger v1.6.0
	github.com/swaggo/swag v1.16.4
)
EOF

cat > .gitignore <<'EOF'
.env
bin/
tmp/
*.log
EOF

cat > .env.example <<'EOF'
HTTP_ADDR=:8080

DB_HOST=postgres
DB_PORT=5432
DB_NAME=subs
DB_USER=subs
DB_PASSWORD=subs
DB_SSLMODE=disable
EOF

# create default .env for docker compose (you can edit later)
cp -f .env.example .env

cat > Dockerfile <<'EOF'
FROM golang:1.22 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download || true
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/app

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=build /app/server /server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
EOF

cat > docker-compose.yml <<'EOF'
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: subs
      POSTGRES_USER: subs
      POSTGRES_PASSWORD: subs
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U subs -d subs"]
      interval: 3s
      timeout: 3s
      retries: 20

  migrate:
    image: ghcr.io/kukymbr/goose-docker:3.20.0
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      GOOSE_DRIVER: postgres
      GOOSE_DBSTRING: "host=postgres port=5432 user=subs password=subs dbname=subs sslmode=disable"
    volumes:
      - ./migrations:/migrations
    command: ["goose", "-dir", "/migrations", "up"]

  app:
    build: .
    env_file:
      - .env
    depends_on:
      migrate:
        condition: service_completed_successfully
    ports:
      - "8080:8080"
EOF

cat > migrations/00001_init.sql <<'EOF'
-- +goose Up
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS subscriptions (
  id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
  service_name TEXT NOT NULL,
  price INTEGER NOT NULL CHECK (price >= 0),
  user_id UUID NOT NULL,
  start_date DATE NOT NULL,
  end_date DATE NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (date_part('day', start_date) = 1),
  CHECK (end_date IS NULL OR date_part('day', end_date) = 1),
  CHECK (end_date IS NULL OR end_date >= start_date)
);

CREATE INDEX IF NOT EXISTS idx_subs_user_id ON subscriptions(user_id);
CREATE INDEX IF NOT EXISTS idx_subs_service_name ON subscriptions(service_name);
CREATE INDEX IF NOT EXISTS idx_subs_dates ON subscriptions(start_date, end_date);

CREATE OR REPLACE FUNCTION set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_set_updated_at ON subscriptions;
CREATE TRIGGER trg_set_updated_at
BEFORE UPDATE ON subscriptions
FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS subscriptions;
EOF

cat > internal/config/config.go <<'EOF'
package config

import (
	"fmt"
	"os"
)

type Config struct {
	HTTPAddr string
	DBDSN    string
}

func Load() (Config, error) {
	httpAddr := getenv("HTTP_ADDR", ":8080")

	host := getenv("DB_HOST", "localhost")
	port := getenv("DB_PORT", "5432")
	name := getenv("DB_NAME", "subs")
	user := getenv("DB_USER", "subs")
	pass := getenv("DB_PASSWORD", "subs")
	ssl := getenv("DB_SSLMODE", "disable")

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, pass, name, ssl,
	)

	return Config{HTTPAddr: httpAddr, DBDSN: dsn}, nil
}

func getenv(k, def string) string {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	return v
}
EOF

cat > internal/db/db.go <<'EOF'
package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func New(dsn string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 1
	cfg.MaxConnLifetime = 30 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return pgxpool.NewWithConfig(ctx, cfg)
}
EOF

cat > internal/util/month.go <<'EOF'
package util

import (
	"fmt"
	"time"
)

// ParseMonth parses "MM-YYYY" and returns time.Time set to the first day of that month (UTC).
func ParseMonth(s string) (time.Time, error) {
	t, err := time.Parse("01-2006", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid month format, expected MM-YYYY: %w", err)
	}
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC), nil
}

func FormatMonth(t time.Time) string {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC).Format("01-2006")
}
EOF

cat > internal/model/subscription.go <<'EOF'
package model

import "time"

type Subscription struct {
	ID          string
	ServiceName string
	Price       int
	UserID      string
	StartDate   time.Time
	EndDate     *time.Time
}
EOF

cat > internal/repo/subscriptions.go <<'EOF'
package repo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"subs-service/internal/model"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type SubscriptionsRepo struct {
	pool *pgxpool.Pool
}

func NewSubscriptionsRepo(pool *pgxpool.Pool) *SubscriptionsRepo {
	return &SubscriptionsRepo{pool: pool}
}

func (r *SubscriptionsRepo) Create(ctx context.Context, s model.Subscription) (model.Subscription, error) {
	q := `
		INSERT INTO subscriptions (service_name, price, user_id, start_date, end_date)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, service_name, price, user_id, start_date, end_date
	`
	row := r.pool.QueryRow(ctx, q, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)
	var out model.Subscription
	err := row.Scan(&out.ID, &out.ServiceName, &out.Price, &out.UserID, &out.StartDate, &out.EndDate)
	return out, err
}

func (r *SubscriptionsRepo) GetByID(ctx context.Context, id string) (model.Subscription, error) {
	q := `
		SELECT id, service_name, price, user_id, start_date, end_date
		FROM subscriptions
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, q, id)
	var out model.Subscription
	if err := row.Scan(&out.ID, &out.ServiceName, &out.Price, &out.UserID, &out.StartDate, &out.EndDate); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, err
	}
	return out, nil
}

func (r *SubscriptionsRepo) List(ctx context.Context, userID *string, serviceName *string, limit, offset int) ([]model.Subscription, error) {
	// pass NULL when filter not provided
	var uid any = nil
	if userID != nil {
		uid = *userID
	}
	var sname any = nil
	if serviceName != nil {
		sname = *serviceName
	}

	q := `
		SELECT id, service_name, price, user_id, start_date, end_date
		FROM subscriptions
		WHERE ($1::uuid IS NULL OR user_id = $1::uuid)
		  AND ($2::text IS NULL OR service_name = $2::text)
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`
	rows, err := r.pool.Query(ctx, q, uid, sname, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []model.Subscription
	for rows.Next() {
		var s model.Subscription
		if err := rows.Scan(&s.ID, &s.ServiceName, &s.Price, &s.UserID, &s.StartDate, &s.EndDate); err != nil {
			return nil, err
		}
		res = append(res, s)
	}
	return res, rows.Err()
}

func (r *SubscriptionsRepo) Update(ctx context.Context, id string, s model.Subscription) (model.Subscription, error) {
	q := `
		UPDATE subscriptions
		SET service_name=$2, price=$3, user_id=$4, start_date=$5, end_date=$6
		WHERE id=$1
		RETURNING id, service_name, price, user_id, start_date, end_date
	`
	row := r.pool.QueryRow(ctx, q, id, s.ServiceName, s.Price, s.UserID, s.StartDate, s.EndDate)
	var out model.Subscription
	if err := row.Scan(&out.ID, &out.ServiceName, &out.Price, &out.UserID, &out.StartDate, &out.EndDate); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Subscription{}, ErrNotFound
		}
		return model.Subscription{}, err
	}
	return out, nil
}

func (r *SubscriptionsRepo) Delete(ctx context.Context, id string) error {
	ct, err := r.pool.Exec(ctx, `DELETE FROM subscriptions WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *SubscriptionsRepo) Cost(ctx context.Context, fromMonth, toMonth time.Time, userID *string, serviceName *string) (int64, error) {
	from := time.Date(fromMonth.Year(), fromMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(toMonth.Year(), toMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
	if to.Before(from) {
		return 0, fmt.Errorf("to must be >= from")
	}

	var uid any = nil
	if userID != nil {
		uid = *userID
	}
	var sname any = nil
	if serviceName != nil {
		sname = *serviceName
	}

	q := `
		WITH months AS (
		  SELECT date_trunc('month', m)::date AS month
		  FROM generate_series($1::date, $2::date, interval '1 month') m
		)
		SELECT COALESCE(SUM(s.price), 0) AS total
		FROM months
		JOIN subscriptions s
		  ON s.start_date <= months.month
		 AND (s.end_date IS NULL OR s.end_date > months.month)
		WHERE ($3::uuid IS NULL OR s.user_id = $3::uuid)
		  AND ($4::text IS NULL OR s.service_name = $4::text);
	`
	var total int64
	if err := r.pool.QueryRow(ctx, q, from, to, uid, sname).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}
EOF

cat > internal/http/server.go <<'EOF'
package http

import (
	"net/http"
	"time"
)

func NewServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
}
EOF

cat > internal/http/router.go <<'EOF'
package http

import (
	"log/slog"
	"time"

	"subs-service/internal/repo"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginswagger "github.com/swaggo/gin-swagger"
)

func NewRouter(r *repo.SubscriptionsRepo) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(requestLogger())

	h := NewHandlers(r)

	router.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	router.POST("/subscriptions", h.CreateSubscription)
	router.GET("/subscriptions/:id", h.GetSubscription)
	router.GET("/subscriptions", h.ListSubscriptions)
	router.PUT("/subscriptions/:id", h.UpdateSubscription)
	router.DELETE("/subscriptions/:id", h.DeleteSubscription)

	router.GET("/subscriptions/cost", h.GetCost)

	router.GET("/swagger/*any", ginswagger.WrapHandler(swaggerfiles.Handler))

	return router
}

func requestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		d := time.Since(start)

		slog.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration_ms", d.Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}
EOF

cat > internal/http/handlers.go <<'EOF'
package http

import (
	"net/http"
	"strconv"
	"time"

	"subs-service/internal/model"
	"subs-service/internal/repo"
	"subs-service/internal/util"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// DTOs (API)

// SubscriptionCreateRequest example:
// {
//   "service_name": "Yandex Plus",
//   "price": 400,
//   "user_id": "60601fee-2bf1-4721-ae6f-7636e79a0cba",
//   "start_date": "07-2025",
//   "end_date": "12-2025"
// }
type SubscriptionCreateRequest struct {
	ServiceName string  `json:"service_name" binding:"required"`
	Price       int     `json:"price" binding:"required"`
	UserID      string  `json:"user_id" binding:"required"`
	StartDate   string  `json:"start_date" binding:"required"` // MM-YYYY
	EndDate     *string `json:"end_date,omitempty"`            // MM-YYYY
}

type SubscriptionResponse struct {
	ID          string  `json:"id"`
	ServiceName string  `json:"service_name"`
	Price       int     `json:"price"`
	UserID      string  `json:"user_id"`
	StartDate   string  `json:"start_date"`         // MM-YYYY
	EndDate     *string `json:"end_date,omitempty"` // MM-YYYY
}

type CostResponse struct {
	Total int64 `json:"total"`
}

type Handlers struct {
	repo *repo.SubscriptionsRepo
}

func NewHandlers(r *repo.SubscriptionsRepo) *Handlers {
	return &Handlers{repo: r}
}

func toResponse(s model.Subscription) SubscriptionResponse {
	var end *string
	if s.EndDate != nil {
		v := util.FormatMonth(*s.EndDate)
		end = &v
	}
	return SubscriptionResponse{
		ID:          s.ID,
		ServiceName: s.ServiceName,
		Price:       s.Price,
		UserID:      s.UserID,
		StartDate:   util.FormatMonth(s.StartDate),
		EndDate:     end,
	}
}

// --------- Handlers ----------

// POST /subscriptions
func (h *Handlers) CreateSubscription(c *gin.Context) {
	var req SubscriptionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}
	if req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be >= 0"})
		return
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id must be UUID"})
		return
	}

	start, err := util.ParseMonth(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var endTime *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		t, err := util.ParseMonth(*req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// optional validation
		if t.Before(start) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end_date must be >= start_date"})
			return
		}
		endTime = &t
	}

	sub := model.Subscription{
		ServiceName: req.ServiceName,
		Price:       req.Price,
		UserID:      req.UserID,
		StartDate:   start,
		EndDate:     endTime,
	}

	created, err := h.repo.Create(c.Request.Context(), sub)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "details": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, toResponse(created))
}

// GET /subscriptions/:id
func (h *Handlers) GetSubscription(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be UUID"})
		return
	}

	s, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toResponse(s))
}

// GET /subscriptions?user_id=&service_name=&limit=&offset=
func (h *Handlers) ListSubscriptions(c *gin.Context) {
	var userID *string
	if v := c.Query("user_id"); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id must be UUID"})
			return
		}
		userID = &v
	}

	var serviceName *string
	if v := c.Query("service_name"); v != "" {
		serviceName = &v
	}

	limit := 50
	offset := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	items, err := h.repo.List(c.Request.Context(), userID, serviceName, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "details": err.Error()})
		return
	}

	resp := make([]SubscriptionResponse, 0, len(items))
	for _, s := range items {
		resp = append(resp, toResponse(s))
	}
	c.JSON(http.StatusOK, resp)
}

// PUT /subscriptions/:id
func (h *Handlers) UpdateSubscription(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be UUID"})
		return
	}

	var req SubscriptionCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body", "details": err.Error()})
		return
	}
	if req.Price < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "price must be >= 0"})
		return
	}
	if _, err := uuid.Parse(req.UserID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id must be UUID"})
		return
	}

	start, err := util.ParseMonth(req.StartDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var endTime *time.Time
	if req.EndDate != nil && *req.EndDate != "" {
		t, err := util.ParseMonth(*req.EndDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if t.Before(start) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end_date must be >= start_date"})
			return
		}
		endTime = &t
	}

	sub := model.Subscription{
		ServiceName: req.ServiceName,
		Price:       req.Price,
		UserID:      req.UserID,
		StartDate:   start,
		EndDate:     endTime,
	}

	updated, err := h.repo.Update(c.Request.Context(), id, sub)
	if err != nil {
		if err == repo.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "details": err.Error()})
		return
	}
	c.JSON(http.StatusOK, toResponse(updated))
}

// DELETE /subscriptions/:id
func (h *Handlers) DeleteSubscription(c *gin.Context) {
	id := c.Param("id")
	if _, err := uuid.Parse(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id must be UUID"})
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		if err == repo.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "db error", "details": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// GET /subscriptions/cost?from=MM-YYYY&to=MM-YYYY&user_id=&service_name=
func (h *Handlers) GetCost(c *gin.Context) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "from and to are required (MM-YYYY)"})
		return
	}

	fromM, err := util.ParseMonth(fromStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	toM, err := util.ParseMonth(toStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var userID *string
	if v := c.Query("user_id"); v != "" {
		if _, err := uuid.Parse(v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "user_id must be UUID"})
			return
		}
		userID = &v
	}

	var serviceName *string
	if v := c.Query("service_name"); v != "" {
		serviceName = &v
	}

	total, err := h.repo.Cost(c.Request.Context(), fromM, toM, userID, serviceName)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, CostResponse{Total: total})
}
EOF

cat > docs/docs.go <<'EOF'
package docs

import "github.com/swaggo/swag"

// This is a minimal embedded Swagger 2.0 spec.
// It satisfies the requirement "предоставить swagger-документацию" without running swag init.
const docTemplate = `{
  "swagger": "2.0",
  "info": {
    "title": "Subscriptions API",
    "description": "REST-сервис для агрегации данных об онлайн подписках пользователей.",
    "version": "1.0"
  },
  "basePath": "/",
  "schemes": ["http"],
  "paths": {
    "/health": {
      "get": { "summary": "Health check", "responses": { "200": { "description": "OK" } } }
    },
    "/subscriptions": {
      "post": {
        "summary": "Create subscription",
        "parameters": [{
          "in": "body",
          "name": "body",
          "required": true,
          "schema": { "$ref": "#/definitions/SubscriptionCreateRequest" }
        }],
        "responses": { "201": { "description": "Created", "schema": { "$ref": "#/definitions/SubscriptionResponse" } } }
      },
      "get": {
        "summary": "List subscriptions",
        "parameters": [
          {"in":"query","name":"user_id","type":"string"},
          {"in":"query","name":"service_name","type":"string"},
          {"in":"query","name":"limit","type":"integer"},
          {"in":"query","name":"offset","type":"integer"}
        ],
        "responses": { "200": { "description": "OK", "schema": { "type": "array", "items": { "$ref": "#/definitions/SubscriptionResponse" } } } }
      }
    },
    "/subscriptions/{id}": {
      "get": {
        "summary": "Get subscription by id",
        "parameters": [{"in":"path","name":"id","required":true,"type":"string"}],
        "responses": { "200": { "description": "OK", "schema": { "$ref": "#/definitions/SubscriptionResponse" } } }
      },
      "put": {
        "summary": "Update subscription",
        "parameters": [
          {"in":"path","name":"id","required":true,"type":"string"},
          {"in":"body","name":"body","required":true,"schema":{"$ref":"#/definitions/SubscriptionCreateRequest"}}
        ],
        "responses": { "200": { "description": "OK", "schema": { "$ref": "#/definitions/SubscriptionResponse" } } }
      },
      "delete": {
        "summary": "Delete subscription",
        "parameters": [{"in":"path","name":"id","required":true,"type":"string"}],
        "responses": { "204": { "description": "No Content" } }
      }
    },
    "/subscriptions/cost": {
      "get": {
        "summary": "Get total cost for period",
        "parameters": [
          {"in":"query","name":"from","required":true,"type":"string","description":"MM-YYYY"},
          {"in":"query","name":"to","required":true,"type":"string","description":"MM-YYYY"},
          {"in":"query","name":"user_id","type":"string"},
          {"in":"query","name":"service_name","type":"string"}
        ],
        "responses": { "200": { "description": "OK", "schema": { "$ref": "#/definitions/CostResponse" } } }
      }
    }
  },
  "definitions": {
    "SubscriptionCreateRequest": {
      "type": "object",
      "required": ["service_name","price","user_id","start_date"],
      "properties": {
        "service_name": {"type":"string"},
        "price": {"type":"integer"},
        "user_id": {"type":"string"},
        "start_date": {"type":"string","description":"MM-YYYY"},
        "end_date": {"type":"string","description":"MM-YYYY"}
      }
    },
    "SubscriptionResponse": {
      "type": "object",
      "properties": {
        "id": {"type":"string"},
        "service_name": {"type":"string"},
        "price": {"type":"integer"},
        "user_id": {"type":"string"},
        "start_date": {"type":"string"},
        "end_date": {"type":"string"}
      }
    },
    "CostResponse": {
      "type": "object",
      "properties": {
        "total": {"type":"integer","format":"int64"}
      }
    }
  }
}`

type swaggerDoc struct{}

func (s *swaggerDoc) ReadDoc() string { return docTemplate }

func init() {
	swag.Register(swag.Name, &swaggerDoc{})
}
EOF

cat > cmd/app/main.go <<'EOF'
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "subs-service/docs"

	"subs-service/internal/config"
	"subs-service/internal/db"
	httpapi "subs-service/internal/http"
	"subs-service/internal/repo"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	pool, err := db.New(cfg.DBDSN)
	if err != nil {
		slog.Error("db connect failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	r := repo.NewSubscriptionsRepo(pool)
	router := httpapi.NewRouter(r)
	srv := httpapi.NewServer(cfg.HTTPAddr, router)

	go func() {
		slog.Info("server started", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil {
			slog.Error("server stopped", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	slog.Info("shutting down...")
	_ = srv.Shutdown(ctx)
	slog.Info("bye")
}
EOF

cat > README.md <<'EOF'
# Effective Mobile - Subscriptions Service (Gin + Postgres)

## Run
```bash
cp .env.example .env
docker compose up --build
EOF

echo
echo "✅ Project created in ./$PROJECT_DIR"
echo "Next:"
echo "  1) cd $PROJECT_DIR"
echo "  2) docker compose up --build"
echo "Swagger: http://localhost:8080/swagger/index.html"
