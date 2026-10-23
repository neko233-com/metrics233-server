package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/neko233-com/metrics233-server/internal/model"
	"net/http"
	"sort"
	"strings"
	"time"
)

// 慢 SQL 文本使用独立、有界的采样契约；不查询业务数据库，也不执行收到的 SQL。
const slowSQLTemplateLimit = 256

type slowSQLVariant struct {
	Template       string  `json:"template"`
	Calls          float64 `json:"calls"`
	Seconds        float64 `json:"seconds"`
	MaximumSeconds float64 `json:"maximum_seconds"`
}
type slowSQLGroup struct {
	ID             string           `json:"id"`
	Table          string           `json:"table"`
	Operation      string           `json:"operation"`
	Namespace      string           `json:"namespace"`
	Template       string           `json:"template"`
	Calls          float64          `json:"calls"`
	Seconds        float64          `json:"seconds"`
	MaximumSeconds float64          `json:"maximum_seconds"`
	Variants       []slowSQLVariant `json:"variants"`
}
type slowSQLTable struct {
	Table     string  `json:"table"`
	Namespace string  `json:"namespace"`
	Calls     float64 `json:"calls"`
	Seconds   float64 `json:"seconds"`
	Groups    int     `json:"groups"`
}

func (s *Server) slowSQLRoutes(api routeRegistrar) {
	api.HandleFunc("GET /api/v1/business/instances/{id}/slow-sql", s.slowSQLSummary)
}

func (s *Server) slowSQLSummary(w http.ResponseWriter, r *http.Request) {
	item, err := s.Store.BusinessInstance(r.Context(), r.PathValue("id"))
	if err != nil {
		businessError(w, err)
		return
	}
	window := r.URL.Query().Get("window")
	if window == "" {
		window = "1h"
	}
	if window != "5m" && window != "15m" && window != "1h" {
		fail(w, 400, errors.New("window must be 5m, 15m or 1h"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	selector := "{" + businessSelector(item) + "}[" + window + "]"
	variantsMap := map[string]*slowSQLGroup{}
	for _, spec := range []struct{ metric, fn string }{{"game_sql_slow_calls_total", "increase"}, {"game_sql_slow_duration_seconds_sum", "increase"}, {"game_sql_slow_duration_seconds_max", "max_over_time"}} {
		rows, queryErr := s.businessVector(ctx, spec.fn+"("+spec.metric+selector+")", time.Now())
		if queryErr != nil {
			fail(w, 503, errors.New("slow SQL query unavailable"))
			return
		}
		for _, row := range rows {
			if row.Values["value"] == nil {
				continue
			}
			template := redactSlowSQL(row.Labels["sql_template"])
			if template == "" {
				continue
			}
			namespace := row.Labels["target_app_id"] + "/" + row.Labels["target_channel"] + "/" + row.Labels["target_environment"]
			if namespace == "//" {
				namespace = item.Identity.Environment + "/" + item.Identity.Channel
			}
			key := model.LabelsJSON(row.Labels)
			entry := variantsMap[key]
			if entry == nil {
				if len(variantsMap) >= slowSQLTemplateLimit {
					fail(w, 422, errors.New("slow SQL result exceeds 256 templates; narrow the window"))
					return
				}
				entry = &slowSQLGroup{Table: row.Labels["table"], Operation: row.Labels["operation"], Namespace: namespace, Template: template}
				variantsMap[key] = entry
			}
			value := *row.Values["value"]
			if value < 0 {
				value = 0
			}
			switch spec.metric {
			case "game_sql_slow_calls_total":
				entry.Calls = value
			case "game_sql_slow_duration_seconds_sum":
				entry.Seconds = value
			default:
				entry.MaximumSeconds = value
			}
		}
	}
	entries := make([]slowSQLGroup, 0, len(variantsMap))
	for _, entry := range variantsMap {
		entries = append(entries, *entry)
	}
	groups := clusterSlowSQL(entries)
	tablesMap := map[string]*slowSQLTable{}
	for _, g := range groups {
		key := g.Namespace + "\x00" + g.Table
		t := tablesMap[key]
		if t == nil {
			t = &slowSQLTable{Table: g.Table, Namespace: g.Namespace}
			tablesMap[key] = t
		}
		t.Groups++
		t.Calls += g.Calls
		t.Seconds += g.Seconds
	}
	tables := make([]slowSQLTable, 0, len(tablesMap))
	for _, table := range tablesMap {
		tables = append(tables, *table)
	}
	sort.Slice(tables, func(i, j int) bool {
		if tables[i].Seconds != tables[j].Seconds {
			return tables[i].Seconds > tables[j].Seconds
		}
		return tables[i].Namespace+tables[i].Table < tables[j].Namespace+tables[j].Table
	})
	writeJSON(w, 200, map[string]any{"groups": groups, "tables": tables, "window": window, "algorithm": "normalized-levenshtein-0.90", "maximum_scope": "process_lifetime", "count_scope": "observed_counter_increase_estimate", "threshold_ms": 200, "template_limit": slowSQLTemplateLimit})
}

func clusterSlowSQL(entries []slowSQLGroup) []slowSQLGroup {
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		return a.Namespace+"\x00"+a.Table+"\x00"+a.Operation+"\x00"+a.Template < b.Namespace+"\x00"+b.Table+"\x00"+b.Operation+"\x00"+b.Template
	})
	groups := []slowSQLGroup{}
	for _, entry := range entries {
		found := -1
		for i, g := range groups {
			if g.Namespace != entry.Namespace || g.Table != entry.Table || g.Operation != entry.Operation {
				continue
			}
			limit := min(max(len(g.Template), len(entry.Template))/10, 24)
			if boundedSQLDistance(g.Template, entry.Template, limit) <= limit {
				found = i
				break
			}
		}
		if found < 0 {
			sum := sha256.Sum256([]byte(entry.Namespace + "\x00" + entry.Table + "\x00" + entry.Operation + "\x00" + entry.Template))
			groups = append(groups, slowSQLGroup{ID: hex.EncodeToString(sum[:12]), Table: entry.Table, Operation: entry.Operation, Namespace: entry.Namespace, Template: entry.Template, Variants: []slowSQLVariant{}})
			found = len(groups) - 1
		}
		g := &groups[found]
		g.Calls += entry.Calls
		g.Seconds += entry.Seconds
		g.MaximumSeconds = max(g.MaximumSeconds, entry.MaximumSeconds)
		g.Variants = append(g.Variants, slowSQLVariant{entry.Template, entry.Calls, entry.Seconds, entry.MaximumSeconds})
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Seconds != groups[j].Seconds {
			return groups[i].Seconds > groups[j].Seconds
		}
		return groups[i].ID < groups[j].ID
	})
	return groups
}

// 有界带状 Levenshtein：超过阈值立即结束，内存不随比较次数累积。
func boundedSQLDistance(a, b string, limit int) int {
	if a == b {
		return 0
	}
	if len(a) == 0 || len(b) == 0 {
		return max(len(a), len(b))
	}
	if len(a) > 512 || len(b) > 512 {
		return limit + 1
	}
	if absSQL(len(a)-len(b)) > limit {
		return limit + 1
	}
	var previousBuffer, currentBuffer [513]int
	previous := previousBuffer[:len(b)+1]
	current := currentBuffer[:len(b)+1]
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		for j := range current {
			current[j] = limit + 1
		}
		current[0] = i
		minimum := limit + 1
		for j := max(1, i-limit); j <= min(len(b), i+limit); j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
			minimum = min(minimum, current[j])
		}
		if minimum > limit {
			return limit + 1
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}
func absSQL(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// 接收方再次脱敏，不信任客户端模板已经安全；不保留任意注释或字符串字面量。
func redactSlowSQL(sql string) string {
	if len(sql) == 0 || len(sql) > 512 {
		return ""
	}
	var result strings.Builder
	for i := 0; i < len(sql); {
		c := sql[i]
		if c == '\'' || c == '"' {
			quote := c
			i++
			closed := false
			for i < len(sql) {
				if sql[i] == '\\' {
					i += 2
					continue
				}
				if sql[i] == quote {
					i++
					if i < len(sql) && sql[i] == quote {
						i++
						continue
					}
					closed = true
					break
				}
				i++
			}
			if !closed {
				return "[unparsed SQL]"
			}
			result.WriteString("?")
			continue
		}
		if c == '#' || c == '-' && i+1 < len(sql) && sql[i+1] == '-' {
			for i < len(sql) && sql[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(sql) && sql[i+1] == '*' {
			end := strings.Index(sql[i+2:], "*/")
			if end < 0 {
				return "[unparsed SQL]"
			}
			i += end + 4
			continue
		}
		if c >= '0' && c <= '9' && (i == 0 || !sqlIdentifierByte(sql[i-1])) {
			i++
			for i < len(sql) && (sqlIdentifierByte(sql[i]) || sql[i] == '.') {
				i++
			}
			result.WriteByte('?')
			continue
		}
		if c >= 128 || c < 32 && c != '\n' && c != '\t' {
			return "[unparsed SQL]"
		}
		result.WriteByte(c)
		i++
	}
	return strings.ToLower(strings.Join(strings.Fields(result.String()), " "))
}
func sqlIdentifierByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
