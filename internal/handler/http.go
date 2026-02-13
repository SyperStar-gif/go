package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"subscriptions/internal/model"
	"subscriptions/internal/repository"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

type Handler struct {
	repo   *repository.SubscriptionRepository
	logger *slog.Logger
}

func New(repo *repository.SubscriptionRepository, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

type subscriptionResponse struct {
	ID          uuid.UUID `json:"id"`
	ServiceName string    `json:"service_name"`
	Price       int       `json:"price"`
	UserID      uuid.UUID `json:"user_id"`
	StartDate   string    `json:"start_date"`
	EndDate     *string   `json:"end_date,omitempty"`
	CreatedAt   string    `json:"created_at"`
	UpdatedAt   string    `json:"updated_at"`
}

func toResponse(s model.Subscription) subscriptionResponse {
	var endDate *string
	if s.EndDate != nil {
		v := model.FormatMonthYear(*s.EndDate)
		endDate = &v
	}

	return subscriptionResponse{
		ID:          s.ID,
		ServiceName: s.ServiceName,
		Price:       s.Price,
		UserID:      s.UserID,
		StartDate:   model.FormatMonthYear(s.StartDate),
		EndDate:     endDate,
		CreatedAt:   s.CreatedAt.Format(http.TimeFormat),
		UpdatedAt:   s.UpdatedAt.Format(http.TimeFormat),
	}
}

func (h *Handler) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(h.loggingMiddleware)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/swagger.yaml", h.getSwagger)

	r.Route("/api/v1/subscriptions", func(r chi.Router) {
		r.Post("/", h.create)
		r.Get("/", h.list)
		r.Get("/total", h.total)
		r.Get("/{id}", h.get)
		r.Put("/{id}", h.update)
		r.Delete("/{id}", h.delete)
	})

	return r
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var payload model.SubscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	subscription, err := payload.Validate()
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.repo.Create(r.Context(), subscription)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusCreated, toResponse(created))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	subscription, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, toResponse(subscription))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var payload model.SubscriptionPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	subscription, err := payload.Validate()
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.repo.Update(r.Context(), id, subscription)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, toResponse(updated))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "subscription not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit := 50
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive number")
			return
		}
		limit = parsed
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		parsed, err := strconv.Atoi(v)
		if err != nil || parsed < 0 {
			h.writeError(w, http.StatusBadRequest, "offset must be >= 0")
			return
		}
		offset = parsed
	}

	userID, err := parseOptionalUUID(r.URL.Query().Get("user_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "user_id must be valid UUID")
		return
	}

	items, err := h.repo.List(r.Context(), userID, r.URL.Query().Get("service_name"), limit, offset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]subscriptionResponse, 0, len(items))
	for _, s := range items {
		result = append(result, toResponse(s))
	}
	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) total(w http.ResponseWriter, r *http.Request) {
	periodStart := r.URL.Query().Get("period_start")
	periodEnd := r.URL.Query().Get("period_end")
	if periodStart == "" || periodEnd == "" {
		h.writeError(w, http.StatusBadRequest, "period_start and period_end are required")
		return
	}

	startDate, err := model.ParseMonthYear(periodStart)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	endDate, err := model.ParseMonthYear(periodEnd)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if endDate.Before(startDate) {
		h.writeError(w, http.StatusBadRequest, "period_end must be >= period_start")
		return
	}

	userID, err := parseOptionalUUID(r.URL.Query().Get("user_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "user_id must be valid UUID")
		return
	}

	total, err := h.repo.TotalCost(r.Context(), startDate, endDate, userID, r.URL.Query().Get("service_name"))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"period_start": periodStart,
		"period_end":   periodEnd,
		"total_price":  total,
	})
}

func parseOptionalUUID(value string) (*uuid.UUID, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (h *Handler) getSwagger(w http.ResponseWriter, _ *http.Request) {
	b, err := os.ReadFile("docs/swagger.yaml")
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, "swagger not found")
		return
	}
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}
