package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/requestip"
	"github.com/neko233-com/metrics233-server/internal/store"
)

type reportingSourceAccess struct{ trusted []*net.IPNet }
type reportingBatch struct {
	accepted int
	service  string
	address  string
}
type reportingBatchKey struct{}

// ConfigureReportingProxies is an explicit startup step, before Handler starts.
// A raw visitor forwarding header can never opt itself into this trust list.
func (s *Server) ConfigureReportingProxies(cidrs []string) error {
	access := &reportingSourceAccess{}
	for _, raw := range cidrs {
		if raw = strings.TrimSpace(raw); raw == "" {
			continue
		}
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return errors.New("invalid trusted proxy CIDR")
		}
		access.trusted = append(access.trusted, network)
	}
	s.reporting = access
	return nil
}

func (s *Server) reportingRoutes(api routeRegistrar) {
	list := func(w http.ResponseWriter, r *http.Request) {
		result, err := s.Store.ReportingSources(r.Context())
		if err != nil {
			fail(w, 503, errors.New("reporting source catalog unavailable"))
			return
		}
		writeJSON(w, 200, result)
	}
	api.HandleFunc("GET /api/v1/reporting-sources", list)
	api.HandleFunc("PUT /api/v1/reporting-sources/policy", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Version       int64  `json:"version"`
			AllowlistOnly *bool  `json:"allowlist_only"`
			Address       string `json:"address"`
			Action        string `json:"action"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.AllowlistOnly == nil {
			fail(w, 400, errors.New("allowlist_only is required"))
			return
		}
		err := s.Store.UpdateReportingPolicy(r.Context(), store.ReportingPolicyUpdate{Version: input.Version, AllowlistOnly: *input.AllowlistOnly, Address: input.Address, Action: input.Action})
		if err != nil {
			status := 400
			if errors.Is(err, store.ErrReportingPolicyConflict) {
				status = 409
			}
			fail(w, status, err)
			return
		}
		list(w, r)
	})
}

func (s *Server) reportingIngest(protocol string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var trusted []*net.IPNet
		if s.reporting != nil {
			trusted = s.reporting.trusted
		}
		address, err := store.CanonicalReportingAddress(requestip.ClientIP(r, trusted))
		if err != nil {
			fail(w, 400, errors.New("cannot determine reporting source address"))
			return
		}
		allowed, err := s.Store.ReportingSourceAllowed(r.Context(), address)
		if err != nil {
			fail(w, 503, errors.New("reporting access policy unavailable"))
			return
		}
		if !allowed {
			fail(w, 403, errors.New("reporting source is blocked or not allowlisted"))
			return
		}
		batch := &reportingBatch{address: address}
		next(w, r.WithContext(context.WithValue(r.Context(), reportingBatchKey{}, batch)))
		if batch.accepted > 0 {
			// Accepted metric writes must not be retried because an auxiliary
			// catalog update failed, including after a client disconnected.
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), time.Second)
			defer cancel()
			if err := s.Store.RecordReportingSource(ctx, address, protocol, batch.service, batch.accepted); err != nil {
				slog.Warn("reporting source observation could not be recorded")
			}
		}
	}
}

func (s *Server) ingestReported(ctx context.Context, samples []model.Sample) error {
	if err := s.Store.Ingest(ctx, samples); err != nil {
		return err
	}
	if batch, ok := ctx.Value(reportingBatchKey{}).(*reportingBatch); ok {
		batch.accepted += len(samples)
		observeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		if err := s.Store.RecordBusinessInstances(observeCtx, samples, batch.address); err != nil {
			slog.Warn("business instance observation could not be recorded")
		}
		cancel()
		// Service names are bounded display hints, never authorization identity.
		if batch.service == "" {
			for _, sample := range samples[:min(len(samples), 16)] {
				for _, key := range []string{"service_name", "service", "job"} {
					value := sample.Labels[key]
					if len(value) > 0 && len(value) <= 128 && !strings.ContainsAny(value, "\r\n\x00") {
						batch.service = value
						break
					}
				}
				if batch.service != "" {
					break
				}
			}
		}
	}
	return nil
}
