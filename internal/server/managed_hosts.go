package server

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/neko233-com/metrics233-server/internal/hostagent"
	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
)

func (s *Server) hostAdmin(w http.ResponseWriter, r *http.Request) bool {
	p, err := identity.ForOrganization(r.Context(), s.OrganizationID())
	if err != nil || !p.Role.AtLeast(identity.RoleAdmin) {
		fail(w, 403, identity.ErrForbidden)
		return false
	}
	return true
}

func hostActor(r *http.Request) string {
	p, _ := identity.FromContext(r.Context())
	if p.Login != "" {
		return p.Login
	}
	return "workspace-admin"
}

var collectorPlatform = regexp.MustCompile(`^(linux|darwin)-(amd64|arm64)$`)
var collectorSHA = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (s *Server) managedHostRoutes(api routeRegistrar, mux *http.ServeMux) {
	api.HandleFunc("GET /api/v1/managed-hosts", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		items, err := s.Store.ManagedHosts(r.Context())
		if err != nil {
			fail(w, 503, errors.New("host catalog unavailable"))
			return
		}
		writeJSON(w, 200, items)
	})
	save := func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		var h model.ManagedHost
		if err := decode(w, r, &h); err != nil {
			fail(w, 400, err)
			return
		}
		if id := r.PathValue("id"); id != "" {
			if h.ID != "" && h.ID != id {
				fail(w, 400, errors.New("host identity mismatch"))
				return
			}
			h.ID = id
		} else if h.ID != "" {
			fail(w, 400, errors.New("create requires an empty host id"))
			return
		}
		if err := h.Validate(); err != nil {
			fail(w, 400, err)
			return
		}
		out, err := s.Store.SaveManagedHost(r.Context(), h, hostActor(r))
		if err != nil {
			status := 503
			if errors.Is(err, store.ErrHostConflict) {
				status = 409
			} else if errors.Is(err, sql.ErrNoRows) {
				status = 404
			}
			fail(w, status, errors.New(http.StatusText(status)))
			return
		}
		writeJSON(w, 200, out)
	}
	api.HandleFunc("POST /api/v1/managed-hosts", save)
	api.HandleFunc("PUT /api/v1/managed-hosts/{id}", save)
	api.HandleFunc("POST /api/v1/managed-hosts/{id}/revoke", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		var input struct {
			Version int64 `json:"version"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		h, _, err := s.Store.ManagedHost(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 404, errors.New("host not found"))
			return
		}
		if h.Version != input.Version {
			fail(w, 409, store.ErrHostConflict)
			return
		}
		result, err := s.Store.RevokeManagedHost(r.Context(), h, hostActor(r))
		if err != nil {
			if errors.Is(err, store.ErrHostConflict) {
				fail(w, 409, store.ErrHostConflict)
			} else {
				fail(w, 503, errors.New("cannot revoke host credential"))
			}
			return
		}
		writeJSON(w, 200, result)
	})
	api.HandleFunc("GET /api/v1/managed-hosts/{id}/events", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		events, err := s.Store.HostEvents(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 503, errors.New("host events unavailable"))
			return
		}
		writeJSON(w, 200, events)
	})
	api.HandleFunc("POST /api/v1/managed-hosts/{id}/fingerprint", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		h, _, err := s.Store.ManagedHost(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 404, errors.New("host not found"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		pin, err := hostagent.Fingerprint(ctx, h)
		if err != nil {
			fail(w, 502, err)
			return
		}
		writeJSON(w, 200, map[string]string{"fingerprint": pin, "state": "observed_not_trusted"})
	})
	api.HandleFunc("POST /api/v1/managed-hosts/{id}/deployment-plan", func(w http.ResponseWriter, r *http.Request) { s.hostDeployment(w, r, false) })
	api.HandleFunc("POST /api/v1/managed-hosts/{id}/deploy", func(w http.ResponseWriter, r *http.Request) { s.hostDeployment(w, r, true) })
	api.HandleFunc("POST /api/v1/managed-hosts/{id}/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		input := struct {
			Language string `json:"language"`
		}{Language: "en"}
		if r.ContentLength != 0 {
			if err := decode(w, r, &input); err != nil {
				fail(w, 400, err)
				return
			}
		}
		if input.Language != "en" && input.Language != "zh" {
			fail(w, 400, errors.New("language must be en or zh"))
			return
		}
		h, _, err := s.Store.ManagedHost(r.Context(), r.PathValue("id"))
		if err != nil {
			fail(w, 404, errors.New("host not found"))
			return
		}
		if h.Target.Kind != "mysql" {
			fail(w, 400, errors.New("the native database dashboard currently requires a MySQL collector"))
			return
		}
		s.dashboardImportMu.Lock()
		defer s.dashboardImportMu.Unlock()
		items, err := s.Store.Dashboards(r.Context())
		if err != nil {
			fail(w, 503, errors.New("dashboard catalog unavailable"))
			return
		}
		candidate := model.MySQLDashboard(h, input.Language)
		for _, item := range items {
			if item.ID == candidate.ID {
				writeJSON(w, 200, item)
				return
			}
		}
		result, err := s.Store.SaveDashboard(r.Context(), candidate)
		if err != nil {
			fail(w, 503, errors.New("cannot create MySQL dashboard"))
			return
		}
		writeJSON(w, 200, result)
	})
	api.HandleFunc("GET /api/v1/managed-host-artifacts/{platform}", func(w http.ResponseWriter, r *http.Request) {
		if !s.hostAdmin(w, r) {
			return
		}
		platform := r.PathValue("platform")
		if !collectorPlatform.MatchString(platform) {
			fail(w, 400, errors.New("unsupported collector platform"))
			return
		}
		path := filepath.Join(s.CollectorArtifacts, "metrics233-collector-"+platform)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 64<<20 {
			fail(w, 404, errors.New("collector artifact unavailable"))
			return
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fail(w, 503, errors.New("cannot read collector artifact"))
			return
		}
		sum := sha256.Sum256(data)
		expected, err := os.ReadFile(path + ".sha256")
		if err != nil || strings.TrimSpace(string(expected)) != hex.EncodeToString(sum[:]) {
			fail(w, 503, errors.New("collector artifact checksum mismatch"))
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Content-SHA256", hex.EncodeToString(sum[:]))
		_, _ = w.Write(data)
	})
	// Independent host credentials never grant browser/account operations. No
	// route below can run shell, update configuration or access another host.
	mux.HandleFunc("GET /agent/v1/hosts/{id}/configuration", s.hostAgentHandler(func(w http.ResponseWriter, r *http.Request) {
		h, secrets, ok := s.hostCredential(w, r)
		if !ok {
			return
		}
		target := h.Target
		if h.Enabled {
			target.SecureSettings = map[string]string{"password": secrets["collector_password"], "bearer_token": secrets["collector_bearer_token"]}
		}
		writeJSON(w, 200, model.HostAgentConfig{Version: h.Version, Enabled: h.Enabled, Target: target})
	}))
	mux.HandleFunc("POST /agent/v1/hosts/{id}/heartbeat", s.hostAgentHandler(s.hostHeartbeat))
	mux.HandleFunc("/agent/", func(w http.ResponseWriter, r *http.Request) {
		s.secureHeaders(w, r)
		fail(w, 404, errors.New("unknown host agent endpoint"))
	})
}

func (s *Server) hostDeployment(w http.ResponseWriter, r *http.Request, install bool) {
	if !s.hostAdmin(w, r) {
		return
	}
	var input struct {
		Version int64 `json:"version"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	h, secrets, err := s.Store.ManagedHost(r.Context(), r.PathValue("id"))
	if err != nil {
		fail(w, 404, errors.New("host not found"))
		return
	}
	if input.Version != h.Version {
		fail(w, 409, store.ErrHostConflict)
		return
	}
	if h.HostKey == "" {
		fail(w, 400, errors.New("verify and save the SSH host fingerprint before deployment"))
		return
	}
	bootstrap := model.HostBootstrap{Server: s.Plugins.RootURL, HostID: h.ID, Token: secrets["agent_token"]}
	if err := hostagent.ValidateBootstrap(bootstrap); err != nil {
		fail(w, 503, errors.New("configure an HTTPS root URL before host enrollment"))
		return
	}
	if !install {
		if err := s.Store.HostEvent(r.Context(), h.ID, hostActor(r), "prepare-local-install", "prepared"); err != nil {
			fail(w, 503, errors.New("cannot audit deployment preparation"))
			return
		}
		// Explicit Admin POST consumed in memory by the LAN installer. The UI
		// never displays this sensitive body and no handler logs it.
		writeJSON(w, 200, map[string]any{"host": h, "secrets": secrets, "bootstrap": bootstrap})
		return
	}
	s.hostDeployMu.Lock()
	if s.hostDeploying[h.ID] {
		s.hostDeployMu.Unlock()
		fail(w, 409, errors.New("host deployment already running"))
		return
	}
	s.hostDeploying[h.ID] = true
	s.hostDeployMu.Unlock()
	defer func() { s.hostDeployMu.Lock(); delete(s.hostDeploying, h.ID); s.hostDeployMu.Unlock() }()
	select {
	case s.hostDeploySlots <- struct{}{}:
		defer func() { <-s.hostDeploySlots }()
	default:
		fail(w, 429, errors.New("deployment capacity reached"))
		return
	}
	if err := s.Store.HostEvent(r.Context(), h.ID, hostActor(r), "install", "started"); err != nil {
		fail(w, 503, errors.New("cannot audit deployment"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	result, err := hostagent.Deploy(ctx, h, secrets, bootstrap, s.CollectorArtifacts)
	status := "installed_awaiting_heartbeat"
	if err != nil {
		status = "failed"
	}
	auditCtx, auditCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer auditCancel()
	_ = s.Store.HostEvent(auditCtx, h.ID, hostActor(r), "install", status)
	if err != nil {
		fail(w, 502, err)
		return
	}
	writeJSON(w, 200, result)
}

func (s *Server) hostHeartbeat(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.hostCredential(w, r)
	if !ok {
		return
	}
	var report model.HostHeartbeat
	if err := decode(w, r, &report); err != nil {
		fail(w, 400, errors.New("invalid host report"))
		return
	}
	if len(report.Version) > 64 || (report.OS != "linux" && report.OS != "darwin" && report.OS != "windows") || (report.Arch != "amd64" && report.Arch != "arm64") || !collectorSHA.MatchString(report.SHA) || report.AppliedVersion < 1 || report.AppliedVersion > h.Version || (report.State != "paused" && report.State != "collecting" && report.State != "collection_error") || len(report.Samples) > 5000 {
		fail(w, 400, errors.New("invalid or oversized host report"))
		return
	}
	if !h.Enabled && len(report.Samples) > 0 {
		fail(w, 409, errors.New("host collection is paused; reload configuration"))
		return
	}
	if len(report.Samples) > 0 && report.AppliedVersion != h.Version {
		fail(w, 409, errors.New("host configuration changed; reload before collecting"))
		return
	}
	for index := range report.Samples {
		sample := &report.Samples[index]
		if h.Target.Kind != "prometheus" && h.Target.Kind != "" && sample.Name != "up" && !strings.HasPrefix(sample.Name, "scrape_") {
			prefix := h.Target.Kind + "_"
			if h.Target.Kind == "postgresql" {
				prefix = "pg_"
			}
			if !strings.HasPrefix(sample.Name, prefix) {
				fail(w, 400, errors.New("metric is outside this collector namespace"))
				return
			}
		}
		if sample.Labels == nil {
			sample.Labels = map[string]string{}
		}
		for _, key := range []string{"service", "role", "serverId", "environment", "channel", "implementation", "ip", "host"} {
			delete(sample.Labels, key)
		}
		for key, value := range h.Target.Labels {
			sample.Labels[key] = value
		}
		sample.Labels["managed_host"] = h.ID
		sample.Labels["host"] = h.Name
		sample.Labels["job"] = h.Target.Name
		sample.Labels["instance"] = h.Target.URL
		if err := sample.Validate(); err != nil {
			fail(w, 400, errors.New("invalid host metric sample"))
			return
		}
	}
	if len(report.Samples) > 0 {
		accepted := false
		s.reportingIngest("json", func(w http.ResponseWriter, r *http.Request) {
			if err := s.ingestReported(r.Context(), report.Samples); err != nil {
				fail(w, 400, errors.New("host metrics were rejected"))
				return
			}
			accepted = true
		})(w, r)
		if !accepted {
			return
		}
	}
	if err := s.Store.RecordHostHeartbeat(r.Context(), h.ID, report, len(report.Samples)); err != nil {
		fail(w, 503, errors.New("host heartbeat unavailable"))
		return
	}
	writeJSON(w, 200, map[string]int{"accepted": len(report.Samples)})
}

func (s *Server) hostCredential(w http.ResponseWriter, r *http.Request) (model.ManagedHost, map[string]string, bool) {
	s.secureHeaders(w, r)
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get("Origin") != "" || r.Header.Get("Cookie") != "" {
		fail(w, 403, errors.New("host credentials are not browser sessions"))
		return model.ManagedHost{}, nil, false
	}
	fields := r.Header.Values("Authorization")
	if len(fields) != 1 || !strings.HasPrefix(fields[0], "Bearer ") {
		fail(w, 401, errors.New("valid host credential required"))
		return model.ManagedHost{}, nil, false
	}
	h, secrets, err := s.Store.AuthenticateHost(r.Context(), r.PathValue("id"), strings.TrimPrefix(fields[0], "Bearer "))
	if err != nil {
		fail(w, 401, errors.New("valid host credential required"))
		return model.ManagedHost{}, nil, false
	}
	return h, secrets, true
}

func (s *Server) hostAgentHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.hostAgentSlots <- struct{}{}:
			defer func() { <-s.hostAgentSlots }()
		default:
			w.Header().Set("Retry-After", "1")
			fail(w, 429, errors.New("agent capacity reached"))
			return
		}
		next(w, r)
	}
}
