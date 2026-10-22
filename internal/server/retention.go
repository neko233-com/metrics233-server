package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/neko233-com/metrics233-server/internal/store"
)

func (s *Server) retentionSetting(ctx context.Context) (store.MetricRetention, error) {
	return s.Store.MetricRetention(ctx, s.RetentionDays)
}

func (s *Server) retentionRoutes(api routeRegistrar) {
	respond := func(w http.ResponseWriter, r *http.Request) {
		value, err := s.retentionSetting(r.Context())
		if err != nil {
			fail(w, 503, errors.New("retention setting unavailable"))
			return
		}
		writeJSON(w, 200, map[string]any{"days": value.Days, "version": value.Version, "editable": s.Store.Kind() == "sqlite" || s.Store.Kind() == "embedded", "cleanup_interval_seconds": 3600, "backend": s.Store.Kind(), "compressed_timeseries": s.Store.Kind() == "embedded", "embedded_vectors": s.Store.Kind() == "embedded"})
	}
	api.HandleFunc("GET /api/v1/settings/retention", respond)
	api.HandleFunc("PUT /api/v1/settings/retention", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Days    *int   `json:"days"`
			Version *int64 `json:"version"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.Days == nil || input.Version == nil {
			fail(w, 400, errors.New("days and version are required"))
			return
		}
		_, err := s.Store.SaveMetricRetention(r.Context(), store.MetricRetention{Days: *input.Days, Version: *input.Version})
		if err != nil {
			status := 400
			if errors.Is(err, store.ErrRetentionConflict) {
				status = 409
			}
			fail(w, status, err)
			return
		}
		respond(w, r)
	})
}

func (s *Server) pruneExpiredMetrics(ctx context.Context, now time.Time) error {
	return s.Store.PruneMetricRetention(ctx, s.RetentionDays, now)
}
