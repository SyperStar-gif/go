package handler

import (
	"encoding/json"
	"net/http"

	"subscriptions/internal/analytics"
)

func (h *Handler) analyticsSimulate(w http.ResponseWriter, r *http.Request) {
	var payload analytics.SimulationRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	report, err := analytics.NewService().GenerateDailyReport(payload)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	h.writeJSON(w, http.StatusOK, report)
}
