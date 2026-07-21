package api

import (
	"net/http"

	"github.com/KB-perByte/hiveshare/internal/store"
)

type MetricsHandler struct {
	metrics *store.MetricsStore
}

func NewMetricsHandler(metrics *store.MetricsStore) *MetricsHandler {
	return &MetricsHandler{metrics: metrics}
}

func (h *MetricsHandler) UserMetrics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	m, err := h.metrics.UserMetrics(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}
