package model

import (
	"encoding/json"
	"errors"
	"github.com/neko233-com/metrics233-server/internal/telemetry"
	"regexp"
	"unicode/utf8"
)

var metricName = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]{0,199}$`)
var labelName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]{0,99}$`)

type Sample = telemetry.Sample
type Target = telemetry.Target

func ValidateLabels(labels map[string]string) error { return telemetry.ValidateLabels(labels) }

type Panel struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	Metric        string            `json:"metric"`
	Aggregation   string            `json:"aggregation"`
	Unit          string            `json:"unit"`
	Labels        map[string]string `json:"labels,omitempty"`
	Expr          string            `json:"expr,omitempty"`
	Expressions   []string          `json:"expressions,omitempty"`
	Visualization string            `json:"visualization,omitempty"`
	// Config retains the Grafana panel contract rather than a lossy conversion.
	Config json.RawMessage `json:"config,omitempty"`
}

type Variable struct {
	Name       string          `json:"name"`
	Type       string          `json:"type"`
	Query      string          `json:"query"`
	Current    string          `json:"current"`
	Options    []string        `json:"options"`
	Multi      bool            `json:"multi"`
	IncludeAll bool            `json:"include_all"`
	Config     json.RawMessage `json:"config,omitempty"`
}

type Dashboard struct {
	ID             string          `json:"id"`
	GrafanaID      int64           `json:"grafana_id,omitempty"`
	GrafanaVersion int64           `json:"grafana_version,omitempty"`
	FolderUID      string          `json:"folder_uid,omitempty"`
	PluginID       string          `json:"plugin_id,omitempty"`
	PluginPath     string          `json:"plugin_path,omitempty"`
	Name           string          `json:"name"`
	Panels         []Panel         `json:"panels"`
	UpdatedAt      int64           `json:"updated_at"`
	Variables      []Variable      `json:"variables,omitempty"`
	Grafana        json.RawMessage `json:"grafana,omitempty"`
	Annotations    json.RawMessage `json:"annotations,omitempty"`
}

func ValidAggregation(a string) bool {
	switch a {
	case "last", "avg", "sum", "min", "max", "rate":
		return true
	}
	return false
}

func (d Dashboard) Validate() error {
	if len(d.Annotations) > 0 {
		var queries []map[string]json.RawMessage
		if len(d.Annotations) > 128*1024 || json.Unmarshal(d.Annotations, &queries) != nil || queries == nil || len(queries) > 32 {
			return errors.New("annotations must be a JSON array under 128 KiB with at most 32 queries")
		}
		for _, query := range queries {
			if query == nil {
				return errors.New("annotation queries must be JSON objects")
			}
		}
	}
	if len(d.Name) == 0 || utf8.RuneCountInString(d.Name) > 5000 {
		return errors.New("dashboard name must contain 1–5000 characters")
	}
	if len(d.Panels) > 500 {
		return errors.New("at most 500 panels per dashboard")
	}
	ids := map[string]bool{}
	for _, p := range d.Panels {
		if p.ID == "" || ids[p.ID] {
			return errors.New("panel IDs must be nonempty and unique")
		}
		ids[p.ID] = true
		if len(p.Title) == 0 || len(p.Title) > 256 || (len(p.Config) == 0 && p.Expr == "" && len(p.Expressions) == 0 && !metricName.MatchString(p.Metric)) || !ValidAggregation(p.Aggregation) {
			return errors.New("each panel needs a title, valid metric and aggregation")
		}
		if len(p.Config) > 512*1024 || (len(p.Config) > 0 && !json.Valid(p.Config)) {
			return errors.New("panel config must be valid JSON under 512 KiB")
		}
		if p.Unit != "" && p.Unit != "bytes" && p.Unit != "seconds" && p.Unit != "percent" && p.Unit != "count" && p.Unit != "ops" && p.Unit != "uptime" {
			return errors.New("unsupported panel unit")
		}
		if err := ValidateLabels(p.Labels); err != nil {
			return err
		}
		if len(p.Expr) > 10000 || len(p.Expressions) > 32 {
			return errors.New("expression limit: 10000 bytes, 32 queries per panel")
		}
		for _, expr := range p.Expressions {
			if len(expr) > 10000 {
				return errors.New("expression exceeds 10000 bytes")
			}
		}
	}
	if len(d.Variables) > 32 {
		return errors.New("at most 32 variables")
	}
	for _, v := range d.Variables {
		if !labelName.MatchString(v.Name) || len(v.Config) > 128*1024 || (len(v.Config) > 0 && !json.Valid(v.Config)) {
			return errors.New("variable needs a valid name and JSON config under 128 KiB")
		}
	}
	return nil
}

func LabelsJSON(labels map[string]string) string {
	if labels == nil {
		return "{}"
	}
	b, _ := json.Marshal(labels)
	return string(b)
}

type Point struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}
type Series struct {
	Labels map[string]string `json:"labels"`
	Points []Point           `json:"points"`
}
type Query struct {
	Metric           string
	Labels           map[string]string
	Start, End, Step int64
	Aggregation      string
}
type QueryResult struct {
	Metric      string   `json:"metric"`
	Aggregation string   `json:"aggregation"`
	Start       int64    `json:"start"`
	End         int64    `json:"end"`
	Step        int64    `json:"step"`
	Series      []Series `json:"series"`
}
