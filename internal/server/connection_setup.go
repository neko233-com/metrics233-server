package server

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/metrics233-server/internal/collector"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
)

type connectionSetupRequest struct {
	Setup    model.ConnectionSetupInput `json:"setup"`
	PlanHash string                     `json:"plan_hash,omitempty"`
}

func connectionRequestSchema(apply bool) map[string]any {
	setup := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "language", "alerts"}, "properties": map[string]any{
		"id":                 map[string]any{"type": "string", "format": "uuid", "description": "Client-generated canonical UUID; reuse for uncertain apply retries."},
		"existing_target_id": map[string]any{"type": "integer", "minimum": 1, "description": "Existing collector: omit target settings; stored credentials/configuration are read and fenced."},
		"language":           map[string]any{"enum": []string{"en", "zh"}}, "alerts": map[string]any{"type": "boolean"}, "receiver": map[string]any{"type": "string", "maxLength": 100, "description": "Optional contact point UID; empty follows existing notification policy."},
		"target": map[string]any{"type": "object", "description": "New collector Target: name,kind,url,interval_seconds=5..86400,enabled=true,labels?,username?,database?,tls_mode?,secure_settings={password?,bearer_token?}. Credentials are write-only. Existing collectors send an empty object or omit this field."},
	}, "oneOf": []any{map[string]any{"required": []string{"existing_target_id"}}, map[string]any{"required": []string{"target"}, "not": map[string]any{"required": []string{"existing_target_id"}}}}}
	required := []string{"setup"}
	if apply {
		required = append(required, "plan_hash")
	}
	return map[string]any{"type": "object", "additionalProperties": false, "required": required, "properties": map[string]any{"setup": setup, "plan_hash": map[string]any{"type": "string", "pattern": "^[0-9a-f]{64}$", "description": "Exact hash returned by preview; required for apply."}}}
}

func (s *Server) connectionRoutes(api routeRegistrar) {
	api.HandleFunc("GET /api/v1/connections", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.Store.ConnectionSetups(r.Context())
		if err != nil {
			fail(w, 503, errors.New("connection catalog unavailable"))
			return
		}
		writeJSON(w, 200, items)
	})
	api.HandleFunc("POST /api/v1/connections/preview", func(w http.ResponseWriter, r *http.Request) { s.connectionSetup(w, r, false) })
	api.HandleFunc("POST /api/v1/connections/apply", func(w http.ResponseWriter, r *http.Request) { s.connectionSetup(w, r, true) })
	api.HandleFunc("POST /api/v1/connections/discover", s.discoverLocalConnections)
}

func (s *Server) connectionSetup(w http.ResponseWriter, r *http.Request, apply bool) {
	if !s.hostAdmin(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var request connectionSetupRequest
	if err := decode(w, r, &request); err != nil {
		fail(w, 400, errors.New("invalid quick connection document"))
		return
	}
	if request.Setup.ExistingTargetID > 0 {
		t := request.Setup.Target
		if t.ID != 0 || t.Name != "" || t.URL != "" || t.Kind != "" || t.Username != "" || t.Database != "" || t.TLSMode != "" || t.Enabled || t.IntervalSeconds != 0 || t.LastScrape != 0 || t.LastError != "" || t.Samples != 0 || t.DurationMS != 0 || len(t.Labels) > 0 || len(t.SecureSettings) > 0 || len(t.SecureFields) > 0 {
			fail(w, 400, errors.New("existing target setup reads stored settings; omit target fields"))
			return
		}
		actual, err := s.Store.ConnectionTargetSnapshot(r.Context(), request.Setup.ExistingTargetID)
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, errors.New("collector not found"))
			return
		}
		if err != nil {
			fail(w, 503, errors.New("collector configuration unavailable"))
			return
		}
		request.Setup.Target = actual
		all, err := s.Store.Targets(r.Context())
		if err != nil {
			fail(w, 503, errors.New("collector catalog unavailable"))
			return
		}
		labels := func(target model.Target) map[string]string {
			out := map[string]string{"job": target.Name, "instance": target.URL}
			for k, v := range target.Labels {
				out[k] = v
			}
			return out
		}
		scope := labels(actual)
		for _, other := range all {
			if other.ID == request.Setup.ExistingTargetID {
				continue
			}
			candidate := labels(other)
			matches := true
			for k, v := range scope {
				if candidate[k] != v {
					matches = false
					break
				}
			}
			if matches {
				fail(w, 409, errors.New("collector labels overlap another target; choose distinct identities before creating a dashboard"))
				return
			}
		}
	}
	if err := request.Setup.Validate(); err != nil {
		fail(w, 400, err)
		return
	}
	if apply {
		if len(request.PlanHash) != 64 || strings.Trim(request.PlanHash, "0123456789abcdef") != "" {
			fail(w, 400, errors.New("apply requires the reviewed plan_hash"))
			return
		}
		receipt, err := s.Store.ConnectionRetry(r.Context(), request.Setup)
		if err == nil {
			writeJSON(w, 200, receipt)
			return
		}
		if !errors.Is(err, sql.ErrNoRows) {
			s.connectionSetupError(w, err)
			return
		}
	}
	select {
	case s.connectionSlots <- struct{}{}:
		defer func() { <-s.connectionSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		fail(w, 429, errors.New("connection probes busy; retry later"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	target := request.Setup.CollectionTarget()
	samples, err := s.Collector.Preview(ctx, target, target.SecureSettings)
	if err != nil {
		fail(w, 502, errors.New("connection test failed; check the address, credentials, TLS and service permissions"))
		return
	}
	plan, err := model.BuildConnectionPlan(request.Setup, samples)
	if err != nil {
		fail(w, 422, errors.New("collector observations could not produce a supported connection plan"))
		return
	}
	plan.PlanHash = s.Store.ConnectionPlanProof(request.Setup, plan.PlanHash)
	if !apply {
		writeJSON(w, 200, plan)
		return
	}
	if request.PlanHash != plan.PlanHash {
		fail(w, 409, errors.New("detected connection plan changed; preview it again before applying"))
		return
	}
	receipt, err := s.Store.ApplyConnectionSetup(r.Context(), request.Setup, plan)
	if err != nil {
		s.connectionSetupError(w, err)
		return
	}
	// The normal collector owns ingestion. Preview samples are never promoted to
	// a fictitious successful scrape or mixed into the control transaction.
	writeJSON(w, 200, receipt)
}

func (s *Server) connectionSetupError(w http.ResponseWriter, err error) {
	if errors.Is(err, store.ErrConnectionConflict) || errors.Is(err, store.ErrRuleConflict) || errors.Is(err, store.ErrResourceExists) {
		fail(w, 409, store.ErrConnectionConflict)
		return
	}
	fail(w, 503, errors.New("connection could not be saved; inspect current state before retrying"))
}

type discoveredConnection struct {
	Preset     string `json:"preset"`
	Name       string `json:"name"`
	URL        string `json:"url"`
	Profile    string `json:"profile"`
	Samples    int    `json:"samples"`
	Configured bool   `json:"configured"`
}

// Discovery is an explicit administrator read of fixed loopback exporter ports.
// It neither scans subnets, tries database passwords nor writes configuration.
func (s *Server) discoverLocalConnections(w http.ResponseWriter, r *http.Request) {
	if !s.hostAdmin(w, r) {
		return
	}
	if r.ContentLength != 0 {
		fail(w, 400, errors.New("local discovery takes no body or arbitrary addresses"))
		return
	}
	if r.URL.RawQuery != "" {
		fail(w, 400, errors.New("local discovery takes no query parameters"))
		return
	}
	select {
	case s.connectionSlots <- struct{}{}:
		defer func() { <-s.connectionSlots }()
	default:
		w.Header().Set("Retry-After", "2")
		fail(w, 429, errors.New("connection probes busy; retry later"))
		return
	}
	targets, err := s.Store.Targets(r.Context())
	if err != nil {
		fail(w, 503, errors.New("collector catalog unavailable"))
		return
	}
	configured := map[string]bool{}
	normalize := func(address string) string {
		u, err := url.Parse(address)
		if err != nil {
			return ""
		}
		if u.Hostname() == "localhost" {
			u.Host = "127.0.0.1:" + u.Port()
		}
		return u.String()
	}
	for _, t := range targets {
		configured[normalize(t.URL)] = true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	seen := map[string]bool{}
	results := []discoveredConnection{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, 4)
	for _, preset := range collector.Exporters() {
		u, err := url.Parse(preset.Endpoint)
		if err != nil || u.Scheme != "http" || u.Path != "/metrics" || u.RawQuery != "" || u.Hostname() != "localhost" || u.Port() == "" {
			continue
		}
		u.Host = "127.0.0.1:" + u.Port()
		endpoint := u.String()
		if seen[endpoint] {
			continue
		}
		seen[endpoint] = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				return
			}
			probe, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			defer cancel()
			target := model.Target{Name: "local-" + preset.ID, Kind: "prometheus", URL: endpoint, IntervalSeconds: 15, Enabled: true}
			samples, err := s.Collector.Preview(probe, target, nil)
			if err != nil || len(samples) == 0 {
				return
			}
			input := model.ConnectionSetupInput{ID: "23300000-0000-4000-8000-000000000001", Target: target, Language: "en"}
			plan, err := model.BuildConnectionPlan(input, samples)
			if err != nil {
				return
			}
			mu.Lock()
			results = append(results, discoveredConnection{Preset: preset.ID, Name: preset.Name, URL: endpoint, Profile: plan.Profile, Samples: len(samples), Configured: configured[endpoint]})
			mu.Unlock()
		}()
	}
	wg.Wait()
	slices.SortFunc(results, func(a, b discoveredConnection) int { return strings.Compare(a.URL, b.URL) })
	writeJSON(w, 200, map[string]any{"scope": "monitoring-server-loopback", "connections": results})
}
