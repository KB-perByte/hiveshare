package api

import (
	"net/http"

	"github.com/KB-perByte/hiveshare/internal/store"
)

// MetricsHandler serves user-scoped analytics endpoints.
type MetricsHandler struct {
	metrics *store.MetricsStore
}

// NewMetricsHandler returns a MetricsHandler backed by the given metrics store.
func NewMetricsHandler(metrics *store.MetricsStore) *MetricsHandler {
	return &MetricsHandler{metrics: metrics}
}

// UserMetrics returns personal contribution and membership statistics for the
// authenticated user.
func (h *MetricsHandler) UserMetrics(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	m, err := h.metrics.UserMetrics(r.Context(), u.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, m)
}
