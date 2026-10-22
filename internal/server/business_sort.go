package server

import (
	"errors"
	"sort"
	"strings"
)

type businessSortRule struct {
	column     string
	descending bool
}

func parseBusinessSort(kind, raw string) ([]businessSortRule, string, error) {
	allowed := map[string]bool{"country": true, "province": true, "city": true, "value": true}
	if kind == "sql" {
		allowed = map[string]bool{"table": true, "operation": true, "avg_ms_window": true, "calls_window": true, "failures_window": true}
	}
	if raw == "" {
		if kind == "sql" {
			raw = "avg_ms_window:desc"
		} else {
			raw = "country:asc,province:asc,city:asc"
		}
	}
	if len(raw) > 160 {
		return nil, "", errors.New("sort exceeds 160 bytes")
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 5 {
		return nil, "", errors.New("at most five sort rules")
	}
	seen := map[string]bool{}
	rules := make([]businessSortRule, 0, len(parts))
	for _, p := range parts {
		pair := strings.Split(p, ":")
		if len(pair) != 2 || !allowed[pair[0]] || seen[pair[0]] || (pair[1] != "asc" && pair[1] != "desc") {
			return nil, "", errors.New("invalid or duplicate sort column/direction")
		}
		seen[pair[0]] = true
		rules = append(rules, businessSortRule{pair[0], pair[1] == "desc"})
	}
	return rules, raw, nil
}

func sortBusinessRows(rows []businessDataRow, rules []businessSortRule) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		for _, rule := range rules {
			cmp := 0
			if rule.column == "country" || rule.column == "province" || rule.column == "city" || rule.column == "table" || rule.column == "operation" {
				cmp = strings.Compare(strings.ToLower(a.Labels[rule.column]), strings.ToLower(b.Labels[rule.column]))
			} else {
				av, bv := a.Values[rule.column], b.Values[rule.column]
				if av == nil || bv == nil {
					if av != bv {
						return av != nil
					}
					continue
				}
				if *av < *bv {
					cmp = -1
				} else if *av > *bv {
					cmp = 1
				}
			}
			if cmp != 0 {
				if rule.descending {
					return cmp > 0
				}
				return cmp < 0
			}
		}
		return a.Key < b.Key
	})
}
