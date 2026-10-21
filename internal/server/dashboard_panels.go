package server

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/neko233-com/metrics233-server/internal/model"
)

// Panel edits accept only fields that the editor controls. Saved query models
// and plugin options are copied on the server, including when duplicating.
type nativePanelEdit struct {
	Action   string `json:"action"`
	SourceID string `json:"sourceId,omitempty"`
	Panel    struct {
		ID            string            `json:"id"`
		Title         string            `json:"title"`
		Visualization string            `json:"visualization"`
		Unit          string            `json:"unit"`
		DisplayUnit   *string           `json:"display_unit,omitempty"`
		Metric        string            `json:"metric"`
		Expr          string            `json:"expr"`
		Aggregation   string            `json:"aggregation"`
		Labels        map[string]string `json:"labels"`
		Thresholds    json.RawMessage   `json:"thresholds,omitempty"`
	} `json:"panel"`
	Position *panelPosition `json:"position,omitempty"`
}

func (s *Server) dashboardPanelRoutes(api routeRegistrar) {
	api.HandleFunc("PATCH /api/v1/dashboards/{id}/panels", func(w http.ResponseWriter, r *http.Request) {
		match := strings.TrimSpace(r.Header.Get("If-Match"))
		if match == "" {
			fail(w, 428, errors.New("If-Match dashboard revision is required"))
			return
		}
		if len(match) >= 2 && match[0] == '"' && match[len(match)-1] == '"' {
			match = match[1 : len(match)-1]
		}
		revision, err := strconv.ParseInt(match, 10, 64)
		if err != nil || revision <= 0 {
			fail(w, 400, errors.New("If-Match must contain a positive dashboard revision"))
			return
		}
		var input nativePanelEdit
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		ctx, err := dashboardSaveContext(r)
		if err != nil {
			fail(w, 400, err)
			return
		}
		d, err := s.Store.Dashboard(ctx, r.PathValue("id"))
		if err != nil {
			dashboardResourceError(w, err)
			return
		}
		if d.UpdatedAt != revision {
			fail(w, 409, errors.New("dashboard changed; reload before editing panels"))
			return
		}
		d, err = applyNativePanelEdit(d, input)
		if err != nil {
			fail(w, 400, err)
			return
		}
		d, err = s.saveDashboardWithLibraryPanels(ctx, d, &revision)
		if err != nil {
			dashboardResourceError(w, err)
			return
		}
		writeJSON(w, 200, d)
	})
}

func applyNativePanelEdit(d model.Dashboard, input nativePanelEdit) (model.Dashboard, error) {
	if len(d.Grafana) > 0 && string(d.Grafana) != "null" {
		return d, errors.New("this panel editor requires a native dashboard")
	}
	if input.Action != "save" && input.Action != "duplicate" && input.Action != "delete" {
		return d, errors.New("panel action must be save, duplicate or delete")
	}
	source := input.SourceID
	if source == "" && input.Action != "duplicate" {
		source = input.Panel.ID
	}
	index := -1
	for i, panel := range d.Panels {
		if panel.ID == source {
			index = i
			break
		}
	}
	if input.Action != "save" && index < 0 {
		return d, errors.New("source panel does not exist")
	}
	if input.Action == "save" && input.SourceID != "" && (index < 0 || input.Panel.ID != source) {
		return d, errors.New("edited panel ID must match its source")
	}
	d.Panels = append([]model.Panel(nil), d.Panels...)
	if input.Action == "delete" {
		d.Panels = append(d.Panels[:index], d.Panels[index+1:]...)
		return d, d.Validate()
	}
	if input.Panel.ID == "" {
		return d, errors.New("panel ID is required")
	}
	if input.Panel.DisplayUnit != nil && (*input.Panel.DisplayUnit != "" && *input.Panel.DisplayUnit != "binBps" || *input.Panel.DisplayUnit == "binBps" && input.Panel.Unit != "bytes") {
		return d, errors.New("display_unit must be empty or binBps; binBps requires unit bytes")
	}
	p := model.Panel{}
	if index >= 0 {
		p = d.Panels[index]
	}
	if input.Action == "duplicate" {
		for _, panel := range d.Panels {
			if panel.ID == input.Panel.ID {
				return d, errors.New("duplicated panel needs a new unique ID")
			}
		}
	}
	config := rawObject{}
	if len(p.Config) > 0 && (json.Unmarshal(p.Config, &config) != nil || config == nil) {
		return d, errors.New("invalid stored panel config")
	}
	if len(config["libraryPanel"]) > 0 && string(config["libraryPanel"]) != "null" {
		return d, errors.New("edit the shared library model before editing this panel")
	}
	oldUnit := p.Unit
	p.ID, p.Title, p.Visualization, p.Unit = input.Panel.ID, input.Panel.Title, input.Panel.Visualization, input.Panel.Unit
	if p.Visualization == "" {
		p.Visualization = "timeseries"
	}
	// SDK queries have their own editor; basic display changes cannot replace them.
	if _, configured := config["targets"]; !configured {
		p.Metric, p.Expr, p.Aggregation, p.Labels = input.Panel.Metric, input.Panel.Expr, input.Panel.Aggregation, input.Panel.Labels
		p.Expressions = nil
		if p.Expr != "" {
			p.Expressions = []string{p.Expr}
		}
	}
	if p.Aggregation == "" {
		p.Aggregation = "last"
	}
	config["type"], config["title"] = rawValue(p.Visualization), rawValue(p.Title)
	var fields, defaults rawObject
	_ = json.Unmarshal(config["fieldConfig"], &fields)
	if fields == nil {
		fields = rawObject{}
	}
	_ = json.Unmarshal(fields["defaults"], &defaults)
	if defaults == nil {
		defaults = rawObject{}
	}
	if len(input.Panel.Thresholds) > 0 {
		if string(input.Panel.Thresholds) == "null" {
			delete(defaults, "thresholds")
			var color struct {
				Mode string `json:"mode"`
			}
			_ = json.Unmarshal(defaults["color"], &color)
			if color.Mode == "thresholds" {
				defaults["color"] = rawValue(map[string]any{"mode": "palette-classic"})
			}
		} else {
			var thresholds struct {
				Warning  *float64 `json:"warning"`
				Critical *float64 `json:"critical"`
			}
			decoder := json.NewDecoder(strings.NewReader(string(input.Panel.Thresholds)))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&thresholds) != nil || thresholds.Warning == nil || thresholds.Critical == nil || math.IsNaN(*thresholds.Warning) || math.IsNaN(*thresholds.Critical) || math.IsInf(*thresholds.Warning, 0) || math.IsInf(*thresholds.Critical, 0) || *thresholds.Critical <= *thresholds.Warning {
				return d, errors.New("thresholds require finite warning and critical numbers with critical greater than warning")
			}
			defaults["thresholds"] = rawValue(map[string]any{"mode": "absolute", "steps": []any{map[string]any{"color": "green", "value": nil}, map[string]any{"color": "yellow", "value": *thresholds.Warning}, map[string]any{"color": "red", "value": *thresholds.Critical}}})
			defaults["color"] = rawValue(map[string]any{"mode": "thresholds"})
		}
	}
	unit := p.Unit
	if unit == "seconds" {
		unit = "s"
	}
	if unit == "count" {
		unit = "short"
	}
	oldDisplayUnit := oldUnit
	if oldDisplayUnit == "seconds" {
		oldDisplayUnit = "s"
	}
	if oldDisplayUnit == "count" {
		oldDisplayUnit = "short"
	}
	var configuredUnit string
	_ = json.Unmarshal(defaults["unit"], &configuredUnit)
	// Omitted display_unit retains opaque imported/custom display units. Basic
	// units still follow an explicit base-unit change when no override existed.
	if index < 0 || len(defaults["unit"]) == 0 || p.Unit != oldUnit && configuredUnit == oldDisplayUnit {
		defaults["unit"] = rawValue(unit)
	}
	if input.Panel.DisplayUnit != nil {
		if *input.Panel.DisplayUnit != "" {
			unit = *input.Panel.DisplayUnit
		}
		defaults["unit"] = rawValue(unit)
	}
	fields["defaults"] = rawValue(defaults)
	config["fieldConfig"] = rawValue(fields)
	if index < 0 || input.Action == "duplicate" || len(config["id"]) == 0 {
		maxID := int64(len(d.Panels))
		for _, other := range d.Panels {
			var otherConfig rawObject
			_ = json.Unmarshal(other.Config, &otherConfig)
			id, _ := strconv.ParseInt(string(otherConfig["id"]), 10, 64)
			if id > maxID {
				maxID = id
			}
		}
		id := maxID + 1
		if index >= 0 && input.Action != "duplicate" {
			id = int64(index + 1)
			for i, other := range d.Panels {
				if i == index {
					continue
				}
				var otherConfig rawObject
				_ = json.Unmarshal(other.Config, &otherConfig)
				otherID, _ := strconv.ParseInt(string(otherConfig["id"]), 10, 64)
				if otherID == id {
					id = maxID + 1
					break
				}
			}
		}
		config["id"] = rawValue(id)
	}
	if input.Position != nil {
		pos := input.Position
		if pos.X < 0 || pos.X > 23 || pos.Y < 0 || pos.Y > 10000 || pos.W < 1 || pos.W > 24-pos.X || pos.H < 1 || pos.H > 100 {
			return d, errors.New("invalid panel position")
		}
		config["gridPos"] = rawValue(pos.grid())
	} else if index < 0 || input.Action == "duplicate" {
		return d, errors.New("new and duplicated panels require a position")
	}
	p.Config = rawValue(config)
	if input.Action == "duplicate" || index < 0 {
		d.Panels = append(d.Panels, p)
	} else {
		d.Panels[index] = p
	}
	return d, d.Validate()
}
