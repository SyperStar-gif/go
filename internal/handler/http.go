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
	repo   *repository.DeliveryRepository
	logger *slog.Logger
}

func New(repo *repository.DeliveryRepository, logger *slog.Logger) *Handler {
	return &Handler{repo: repo, logger: logger}
}

type deliveryResponse struct {
	ID                 uuid.UUID `json:"id"`
	OrderNumber        string    `json:"order_number"`
	CustomerID         uuid.UUID `json:"customer_id"`
	DestinationAddress string    `json:"destination_address"`
	Status             string    `json:"status"`
	Cost               int       `json:"cost"`
	DeliveryDate       string    `json:"delivery_date"`
	CompletedAt        *string   `json:"completed_at,omitempty"`
	CreatedAt          string    `json:"created_at"`
	UpdatedAt          string    `json:"updated_at"`
}

func toResponse(d model.Delivery) deliveryResponse {
	var completedAt *string
	if d.CompletedAt != nil {
		v := model.FormatDate(*d.CompletedAt)
		completedAt = &v
	}

	return deliveryResponse{
		ID:                 d.ID,
		OrderNumber:        d.OrderNumber,
		CustomerID:         d.CustomerID,
		DestinationAddress: d.DestinationAddress,
		Status:             d.Status,
		Cost:               d.Cost,
		DeliveryDate:       model.FormatDate(d.DeliveryDate),
		CompletedAt:        completedAt,
		CreatedAt:          d.CreatedAt.Format(http.TimeFormat),
		UpdatedAt:          d.UpdatedAt.Format(http.TimeFormat),
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

	r.Route("/api/v1/deliveries", func(r chi.Router) {
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
	var payload model.DeliveryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	delivery, err := payload.Validate()
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := h.repo.Create(r.Context(), delivery)
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

	delivery, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "delivery not found")
			return
		}
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, toResponse(delivery))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	var payload model.DeliveryPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	delivery, err := payload.Validate()
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := h.repo.Update(r.Context(), id, delivery)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.writeError(w, http.StatusNotFound, "delivery not found")
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
			h.writeError(w, http.StatusNotFound, "delivery not found")
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

	customerID, err := parseOptionalUUID(r.URL.Query().Get("customer_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "customer_id must be valid UUID")
		return
	}

	items, err := h.repo.List(r.Context(), customerID, r.URL.Query().Get("status"), limit, offset)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	result := make([]deliveryResponse, 0, len(items))
	for _, d := range items {
		result = append(result, toResponse(d))
	}
	h.writeJSON(w, http.StatusOK, result)
}

func (h *Handler) total(w http.ResponseWriter, r *http.Request) {
	fromDateRaw := r.URL.Query().Get("date_from")
	toDateRaw := r.URL.Query().Get("date_to")
	if fromDateRaw == "" || toDateRaw == "" {
		h.writeError(w, http.StatusBadRequest, "date_from and date_to are required")
		return
	}

	fromDate, err := model.ParseDate(fromDateRaw)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	toDate, err := model.ParseDate(toDateRaw)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if toDate.Before(fromDate) {
		h.writeError(w, http.StatusBadRequest, "date_to must be >= date_from")
		return
	}

	customerID, err := parseOptionalUUID(r.URL.Query().Get("customer_id"))
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "customer_id must be valid UUID")
		return
	}

	total, err := h.repo.TotalCost(r.Context(), fromDate, toDate, customerID, r.URL.Query().Get("status"))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"date_from":  fromDateRaw,
		"date_to":    toDateRaw,
		"total_cost": total,
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
