package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/neko233-com/metrics233-server/internal/model"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

type Matcher struct{ Name, Value, Type string }
type RawSeries struct {
	Name   string
	Labels map[string]string
	Points []model.Point
}

// SelectSeries returns only label sets. Discovery must not load every metric
// sample: the number of labels is independent of the retained sample volume.
func (s *Store) SelectSeries(ctx context.Context, start, end int64, matchers []Matcher) ([]RawSeries, error) {
	if s.Backend != nil {
		return s.Backend.SelectSeries(ctx, start, end, matchers)
	}
	candidates, err := selectSQLiteSeries(ctx, s.DB, start, end, matchers)
	if err != nil {
		return nil, err
	}
	out := make([]RawSeries, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, RawSeries{Name: candidate.name, Labels: candidate.labels})
	}
	SortSeries(out)
	return out, nil
}

type sqliteSelectedSeries struct {
	id     int64
	name   string
	labels map[string]string
}

// Match the small series catalog first. Nonmatching series can never consume
// the raw sample budget, even when a regex/negative selector cannot use SQL.
func selectSQLiteSeries(ctx context.Context, db metricQueryer, start, end int64, matchers []Matcher) ([]sqliteSelectedSeries, error) {
	compiled, err := compileMatchers(matchers)
	if err != nil {
		return nil, err
	}
	query := `SELECT id,name,labels FROM series WHERE EXISTS (SELECT 1 FROM samples WHERE samples.series_id=series.id AND timestamp>=? AND timestamp<=?)`
	args := []any{start, end}
	for _, m := range matchers {
		if m.Name == "__name__" && m.Type == "=" {
			query += ` AND name=?`
			args = append(args, m.Value)
		}
	}
	rows, err := db.QueryContext(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []sqliteSelectedSeries{}
	for rows.Next() {
		var id int64
		var name, raw string
		if err = rows.Scan(&id, &name, &raw); err != nil {
			return nil, err
		}
		var labels map[string]string
		if err = json.Unmarshal([]byte(raw), &labels); err != nil {
			return nil, err
		}
		if matchesCompiled(name, labels, compiled) {
			if len(out) >= 10000 {
				return nil, errors.New("discovery exceeds 10000 series; narrow selectors")
			}
			out = append(out, sqliteSelectedSeries{id: id, name: name, labels: labels})
		}
	}
	return out, rows.Err()
}

func Matches(name string, labels map[string]string, matchers []Matcher) bool {
	compiled, err := compileMatchers(matchers)
	return err == nil && matchesCompiled(name, labels, compiled)
}

type compiledMatcher struct {
	Matcher
	expression *promlabels.FastRegexMatcher
}

func compileMatchers(matchers []Matcher) ([]compiledMatcher, error) {
	out := make([]compiledMatcher, 0, len(matchers))
	for _, matcher := range matchers {
		item := compiledMatcher{Matcher: matcher}
		switch matcher.Type {
		case "=", "!=":
		case "=~", "!~":
			var err error
			// Reuse upstream anchoring, DotNL and inline-flag semantics. A plain
			// regexp wrapper disagrees with PromQL for valid multiline labels.
			item.expression, err = promlabels.NewFastRegexMatcher(matcher.Value)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("invalid series matcher")
		}
		out = append(out, item)
	}
	return out, nil
}

func matchesCompiled(name string, labels map[string]string, matchers []compiledMatcher) bool {
	for _, m := range matchers {
		value := labels[m.Name]
		if m.Name == "__name__" {
			value = name
		}
		switch m.Type {
		case "=":
			if value != m.Value {
				return false
			}
		case "!=":
			if value == m.Value {
				return false
			}
		case "=~", "!~":
			match := m.expression.MatchString(value)
			if m.Type == "=~" && !match || m.Type == "!~" && match {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func (s *Store) LoadSeries(ctx context.Context, start, end int64, matchers []Matcher) ([]RawSeries, error) {
	if s.Backend != nil {
		return s.Backend.LoadSeries(ctx, start, end, matchers)
	}
	tx, err := s.DB.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	candidates, err := selectSQLiteSeries(ctx, tx, start, end, matchers)
	if err != nil {
		return nil, err
	}
	out := make([]RawSeries, 0, len(candidates))
	if len(candidates) == 0 {
		return out, nil
	}
	ids := make([]int64, 0, len(candidates))
	positions := make(map[int64]int, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.id)
		positions[candidate.id] = len(out)
		out = append(out, RawSeries{Name: candidate.name, Labels: candidate.labels, Points: []model.Point{}})
	}
	where, args := sqliteSampleRange(ids, start, end)
	rows, err := tx.QueryContext(ctx, `SELECT series_id,timestamp,value FROM samples WHERE `+where+` ORDER BY series_id,timestamp LIMIT 1000001`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lastID := int64(-1)
	index := -1
	count := 0
	for rows.Next() {
		count++
		if count > 1000000 {
			return nil, errors.New("PromQL selection exceeds 1000000 samples; narrow time range or selectors")
		}
		var id, ts int64
		var v float64
		if err = rows.Scan(&id, &ts, &v); err != nil {
			return nil, err
		}
		if id != lastID {
			lastID = id
			index = positions[id]
		}
		if index >= 0 {
			out[index].Points = append(out[index].Points, model.Point{Timestamp: ts, Value: v})
		}
	}
	return out, rows.Err()
}
func SortSeries(series []RawSeries) {
	sort.Slice(series, func(i, j int) bool {
		return series[i].Name+model.LabelsJSON(series[i].Labels) < series[j].Name+model.LabelsJSON(series[j].Labels)
	})
}
