package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/metrics233-server/internal/identity"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/prometheus/prometheus/promql"
)

func businessSelector(item model.BusinessInstance) string {
	keys := []string{"environment", "channel", "service", "role", "serverId", "instance", "implementation"}
	labels := item.Identity.Labels()
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+strconv.Quote(labels[key]))
	}
	return strings.Join(parts, ",")
}

func businessCurrentPopulation(selector string) string {
	population := "game_players_online{" + selector + "}"
	return businessAtSnapshot("game_players_online", selector, "scalar(max(timestamp("+population+")))")
}

func businessAtSnapshot(metric, selector, timestamp string) string {
	metric += "{" + selector + "}"
	return metric + " and (timestamp(" + metric + ") == " + timestamp + ")"
}

type businessPopulation struct {
	Available  bool     `json:"available"`
	Players    *float64 `json:"players"`
	SnapshotAt int64    `json:"snapshot_at"`
	Stale      bool     `json:"stale"`
	Status     string   `json:"status"`
}

func (s *Server) businessPopulation(ctx context.Context, item model.BusinessInstance, at time.Time) (businessPopulation, error) {
	population := businessPopulation{Stale: true, Status: "missing"}
	metric := "game_players_online{" + businessSelector(item) + "}"
	timestamps, err := s.businessVector(ctx, "max(timestamp("+metric+"))", at)
	if err != nil {
		return population, err
	}
	if len(timestamps) == 0 {
		return population, nil
	}
	latest := *timestamps[0].Values["value"]
	population.SnapshotAt = int64(math.Round(latest * 1000))
	values, err := s.businessVector(ctx, metric+" and (timestamp("+metric+") == "+strconv.FormatFloat(latest, 'g', -1, 64)+")", at)
	if err != nil {
		return population, err
	}
	// Equal timestamps do not make different label sets one authoritative
	// player population. Refuse ambiguous snapshots rather than adding them.
	if len(values) != 1 {
		population.Status = "ambiguous"
		return population, nil
	}
	value := *values[0].Values["value"]
	if value < 0 {
		population.Status = "invalid"
		return population, nil
	}
	population.Players = &value
	population.Available = true
	population.Status = "ready"
	population.Stale = time.Now().UnixMilli()-population.SnapshotAt > 90000
	return population, nil
}

func (s *Server) writeBusinessDashboard(w http.ResponseWriter, r *http.Request, item model.BusinessInstance, d model.Dashboard) {
	payload := map[string]any{"instance": item, "dashboard": d}
	if item.Identity.Role != "gateway" {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		population, err := s.businessPopulation(ctx, item, time.Now())
		if err != nil {
			population = businessPopulation{Stale: true, Status: "query_error"}
		}
		payload["population"] = population
	}
	writeJSON(w, 200, payload)
}

func businessDefaultDashboard(item model.BusinessInstance) model.Dashboard {
	d := model.Dashboard{ID: "business-" + item.ID, Name: item.Identity.Service + " · " + item.Identity.Role + " · " + item.Identity.ServerID, UpdatedAt: item.LastSeen, Panels: []model.Panel{
		{ID: "online", Title: "Online players", Metric: "game_players_online", Expr: `sum(game_players_online{${business_scope:raw}})`, Aggregation: "last", Visualization: "stat", Unit: "count"},
		{ID: "sessions", Title: "Successful player sessions · cumulative", Metric: "game_player_sessions_started_total", Expr: `sum(game_player_sessions_started_total{${business_scope:raw}})`, Aggregation: "last", Visualization: "stat", Unit: "count"},
		{ID: "players-trend", Title: "Online players over time", Metric: "game_players_online", Expr: `sum(game_players_online{${business_scope:raw}})`, Aggregation: "last", Visualization: "timeseries", Unit: "count"},
	}}
	current := businessCurrentPopulation("${business_scope:raw}")
	d.Panels[0].Expr = "sum((" + current + ") >= 0) and (count(" + current + ") == 1)"
	d.Panels[2].Expr = d.Panels[0].Expr
	population := "game_players_online{${business_scope:raw}}"
	sessions := businessAtSnapshot("game_player_sessions_started_total", "${business_scope:raw}", "scalar(max(timestamp("+population+")))")
	d.Panels[1].Expr = "sum(" + sessions + ") and (count(" + current + ") == 1)"
	if item.Identity.Role == "gateway" {
		d.Panels = []model.Panel{}
	}
	return businessFixedVariables(d, item)
}

func businessFixedVariables(d model.Dashboard, item model.BusinessInstance) model.Dashboard {
	fixed := item.Identity.Labels()
	fixed["business_scope"] = businessSelector(item)
	variables := make([]model.Variable, 0, len(d.Variables)+len(fixed))
	for _, v := range d.Variables {
		if _, ok := fixed[v.Name]; !ok {
			variables = append(variables, v)
		}
	}
	keys := make([]string, 0, len(fixed))
	for key := range fixed {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := fixed[key]
		// Existing frozen-query variables preserve literal label values (including
		// '$') without interpolating constant.query or creating false dependencies.
		config, _ := json.Marshal(map[string]any{"hide": 2, "refresh": 0, "skipUrlSync": true, "allowCustomValue": false, "options": []map[string]string{{"text": value, "value": value}}})
		variables = append(variables, model.Variable{Name: key, Type: "query", Current: value, Options: []string{value}, Config: config})
	}
	d.Variables = variables
	return d
}

func (s *Server) businessRoutes(api routeRegistrar) {
	api.HandleFunc("GET /api/v1/business/instances", func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		var err error
		if raw := r.URL.Query().Get("limit"); raw != "" {
			limit, err = strconv.Atoi(raw)
		}
		if err != nil {
			fail(w, 400, errors.New("invalid limit"))
			return
		}
		page, err := s.Store.FindBusinessInstances(r.Context(), r.URL.Query().Get("cursor"), r.URL.Query().Get("q"), limit)
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, page)
	})
	api.HandleFunc("GET /api/v1/business/instances/{id}", func(w http.ResponseWriter, r *http.Request) {
		item, err := s.Store.BusinessInstance(r.Context(), r.PathValue("id"))
		if err != nil {
			businessError(w, err)
			return
		}
		writeJSON(w, 200, item)
	})
	api.HandleFunc("PUT /api/v1/business/instances/{id}/dashboard", func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Version     *int64  `json:"version"`
			Mode        *string `json:"mode"`
			DashboardID *string `json:"dashboard_id"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.Version == nil || input.Mode == nil || input.DashboardID == nil {
			fail(w, 400, errors.New("version, mode and dashboard_id are required"))
			return
		}
		binding := model.BusinessBinding{Version: *input.Version, Mode: *input.Mode, DashboardID: *input.DashboardID}
		if binding.DashboardID != "" {
			dashboard, err := s.Store.Dashboard(r.Context(), binding.DashboardID)
			if err != nil {
				businessError(w, err)
				return
			}
			if len(dashboard.Grafana) > 0 {
				fail(w, 400, errors.New("business templates require a native dashboard"))
				return
			}
			if len(businessFixedVariables(dashboard, model.BusinessInstance{}).Variables) > 32 {
				fail(w, 400, errors.New("template has too many variables after adding the fixed business scope (maximum 32)"))
				return
			}
		}
		item, err := s.Store.BindBusinessDashboard(r.Context(), r.PathValue("id"), binding)
		if err != nil {
			businessError(w, err)
			return
		}
		writeJSON(w, 200, item)
	})
	api.HandleFunc("GET /api/v1/business/instances/{id}/dashboard", func(w http.ResponseWriter, r *http.Request) {
		item, err := s.Store.BusinessInstance(r.Context(), r.PathValue("id"))
		if err != nil {
			businessError(w, err)
			return
		}
		d := businessDefaultDashboard(item)
		if item.Binding.Mode != "default" {
			d, err = s.Store.Dashboard(r.Context(), item.Binding.DashboardID)
			if err != nil {
				businessError(w, err)
				return
			}
			d, allowed, err := s.readableDashboard(r.Context(), d, true)
			if err != nil {
				writeResourceReadError(w, err)
				return
			}
			if !allowed {
				fail(w, 403, identity.ErrForbidden)
				return
			}
			resolved, err := s.resolveDashboardLibrary(r.Context(), d)
			if err != nil {
				businessError(w, err)
				return
			}
			d = resolved
			if len(d.Grafana) > 0 {
				fail(w, 400, errors.New("business templates require a native dashboard"))
				return
			}
			s.writeBusinessDashboard(w, r, item, businessFixedVariables(d, item))
			return
		}
		s.writeBusinessDashboard(w, r, item, d)
	})
	api.HandleFunc("GET /api/v1/business/instances/{id}/data", s.businessData)
}

func businessError(w http.ResponseWriter, err error) {
	status := 400
	if errors.Is(err, sql.ErrNoRows) {
		status = 404
	}
	if errors.Is(err, store.ErrBusinessBindingConflict) {
		status = 409
	}
	if errors.Is(err, identity.ErrForbidden) {
		status = 403
	}
	fail(w, status, err)
}

type businessDataRow struct {
	Key    string              `json:"key"`
	Labels map[string]string   `json:"labels"`
	Values map[string]*float64 `json:"values"`
}
type businessDataCursor struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Window string `json:"window"`
	At     int64  `json:"at"`
	Offset int    `json:"offset"`
}

func (s *Server) businessVector(ctx context.Context, expr string, at time.Time) ([]businessDataRow, error) {
	query, err := s.Prometheus.Engine.NewInstantQuery(ctx, s.Prometheus.Queryable, promql.NewPrometheusQueryOpts(false, 0), expr, at)
	if err != nil {
		return nil, err
	}
	defer query.Close()
	result := query.Exec(ctx)
	if result.Err != nil {
		return nil, result.Err
	}
	vector, ok := result.Value.(promql.Vector)
	if !ok {
		return nil, errors.New("expected instant metric vector")
	}
	if len(vector) > 10000 {
		return nil, errors.New("business result exceeds 10000 series; narrow the instance scope")
	}
	rows := make([]businessDataRow, 0, len(vector))
	for _, sample := range vector {
		if sample.H != nil || math.IsNaN(sample.F) || math.IsInf(sample.F, 0) {
			continue
		}
		labels := map[string]string{}
		for _, key := range []string{"country", "province", "city", "table", "operation"} {
			if value := sample.Metric.Get(key); value != "" {
				labels[key] = value
			}
		}
		key := model.LabelsJSON(labels)
		value := sample.F
		rows = append(rows, businessDataRow{Key: key, Labels: labels, Values: map[string]*float64{"value": &value}})
	}
	return rows, nil
}

func (s *Server) businessData(w http.ResponseWriter, r *http.Request) {
	item, err := s.Store.BusinessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		businessError(w, err)
		return
	}
	kind := r.URL.Query().Get("kind")
	if kind != "regions" && kind != "sql" {
		fail(w, 400, errors.New("kind must be regions or sql"))
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "5m"
	}
	if window != "5m" && window != "15m" && window != "1h" {
		fail(w, 400, errors.New("window must be 5m, 15m or 1h"))
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if err != nil || limit < 1 || limit > 100 {
		fail(w, 400, errors.New("limit must be 1–100"))
		return
	}
	cursor := businessDataCursor{ID: item.ID, Kind: kind, Window: window, At: time.Now().UnixMilli()}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		if len(raw) > 1024 {
			fail(w, 400, errors.New("invalid business cursor"))
			return
		}
		data, decodeErr := base64.RawURLEncoding.DecodeString(raw)
		if decodeErr != nil || json.Unmarshal(data, &cursor) != nil || cursor.ID != item.ID || cursor.Kind != kind || cursor.Window != window || cursor.Offset < 0 || cursor.Offset > 10000 || cursor.At < time.Now().Add(-15*time.Minute).UnixMilli() || cursor.At > time.Now().Add(time.Second).UnixMilli() {
			fail(w, 400, errors.New("invalid or expired business cursor; refresh the table"))
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	scope := "{" + businessSelector(item) + "}"
	at := time.UnixMilli(cursor.At)
	rows := []businessDataRow{}
	var population *businessPopulation
	regionsComplete := false
	regionTotal := 0.0
	if kind == "regions" {
		current, popErr := s.businessPopulation(ctx, item, at)
		population = &current
		err = popErr
		if err == nil && current.Available {
			metric := "game_players_online_by_region" + scope
			rows, err = s.businessVector(ctx, "sum by(country,province,city)("+metric+" and (timestamp("+metric+") == "+strconv.FormatFloat(float64(current.SnapshotAt)/1000, 'g', -1, 64)+"))", at)
			if err == nil {
				for _, row := range rows {
					regionTotal += *row.Values["value"]
				}
				regionsComplete = math.Abs(regionTotal-*current.Players) < 1e-6
				if *current.Players == 0 {
					rows = []businessDataRow{}
				}
			}
		}
		if err == nil {
			sort.Slice(rows, func(i, j int) bool { return rows[i].Key < rows[j].Key })
		}
	} else {
		current, popErr := s.businessPopulation(ctx, item, at)
		population = &current
		err = popErr
		currentMetric := func(metric string) string {
			return businessAtSnapshot(metric, businessSelector(item), strconv.FormatFloat(float64(current.SnapshotAt)/1000, 'g', -1, 64))
		}
		metrics := []struct{ metric, column string }{{"game_sql_calls_total", "calls"}, {"game_sql_failures_total", "failures"}, {"game_sql_rows_affected_total", "rows"}}
		joined := map[string]*businessDataRow{}
		add := func(expr, column string) bool {
			values, queryErr := s.businessVector(ctx, expr, at)
			if queryErr != nil {
				err = queryErr
				return false
			}
			for _, value := range values {
				row := joined[value.Key]
				if row == nil {
					row = &businessDataRow{Key: value.Key, Labels: value.Labels, Values: map[string]*float64{}}
					joined[value.Key] = row
				}
				row.Values[column] = value.Values["value"]
			}
			return true
		}
		for _, metric := range metrics {
			if err != nil {
				break
			}
			if current.Available && !add("sum by(table,operation)("+currentMetric(metric.metric)+")", metric.column+"_total") {
				break
			}
			if !add("sum by(table,operation)(increase("+metric.metric+scope+"["+window+"]))", metric.column+"_window") {
				break
			}
		}
		if err == nil {
			add("1000 * sum by(table,operation)(rate(game_sql_duration_seconds_sum"+scope+"["+window+"])) / sum by(table,operation)(rate(game_sql_duration_seconds_count"+scope+"["+window+"]))", "avg_ms_window")
		}
		if err == nil && current.Available {
			add("1000 * sum by(table,operation)("+currentMetric("game_sql_duration_seconds_sum")+") / sum by(table,operation)("+currentMetric("game_sql_duration_seconds_count")+")", "avg_ms_total")
		}
		if err == nil {
			for _, row := range joined {
				rows = append(rows, *row)
			}
			sort.Slice(rows, func(i, j int) bool {
				a, b := rows[i].Values["avg_ms_window"], rows[j].Values["avg_ms_window"]
				if a == nil || b == nil {
					if a != b {
						return a != nil
					}
				} else if *a != *b {
					return *a > *b
				}
				return rows[i].Key < rows[j].Key
			})
		}
	}
	if err != nil {
		fail(w, 422, errors.New("business metric query failed; check query budget and metric availability"))
		return
	}
	total := len(rows)
	start := min(cursor.Offset, total)
	end := min(start+limit, total)
	next := ""
	if end < total {
		cursor.Offset = end
		data, _ := json.Marshal(cursor)
		next = base64.RawURLEncoding.EncodeToString(data)
	}
	payload := map[string]any{"items": rows[start:end], "total": total, "next_cursor": next, "evaluated_at": cursor.At, "window": window}
	payload["population"] = population
	if kind == "regions" {
		payload["regions_complete"] = regionsComplete
		payload["region_total"] = regionTotal
	}
	writeJSON(w, 200, payload)
}
