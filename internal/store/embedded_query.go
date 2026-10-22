package store

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/prometheus/prometheus/model/labels"
	promstorage "github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func embeddedMatchers(input []Matcher) ([]*labels.Matcher, error) {
	out := make([]*labels.Matcher, 0, len(input)+1)
	for _, m := range input {
		var kind labels.MatchType
		switch m.Type {
		case "=":
			kind = labels.MatchEqual
		case "!=":
			kind = labels.MatchNotEqual
		case "=~":
			kind = labels.MatchRegexp
		case "!~":
			kind = labels.MatchNotRegexp
		default:
			return nil, errors.New("invalid series matcher")
		}
		matcher, err := labels.NewMatcher(kind, m.Name, m.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, matcher)
	}
	if len(out) == 0 {
		out = append(out, labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, ""))
	}
	return out, nil
}

type embeddedQuerier struct {
	promstorage.Querier
	release  func()
	once     sync.Once
	closeErr error
}

func (q *embeddedQuerier) Close() error {
	q.once.Do(func() { q.closeErr = q.Querier.Close(); q.release() })
	return q.closeErr
}

// Querier lets PromQL consume compressed chunks directly rather than loading and
// copying every observation through the compatibility RawSeries adapter.
func (b *Embedded) Querier(start, end int64) (promstorage.Querier, error) {
	b.life.RLock()
	if b.closed.Load() {
		b.life.RUnlock()
		return nil, errors.New("embedded storage is closed")
	}
	start = max(start, time.Now().Add(-time.Duration(b.days.Load())*24*time.Hour).UnixMilli())
	q, err := b.db.Querier(start, end)
	if err != nil {
		b.life.RUnlock()
		return nil, err
	}
	return &embeddedQuerier{Querier: q, release: b.life.RUnlock}, nil
}

func seriesIdentity(ls labels.Labels) (string, map[string]string) {
	name := ls.Get(labels.MetricName)
	values := ls.Map()
	delete(values, labels.MetricName)
	return name, values
}

func (b *Embedded) readSeries(ctx context.Context, start, end int64, input []Matcher, values bool) ([]RawSeries, error) {
	ms, err := embeddedMatchers(input)
	if err != nil {
		return nil, err
	}
	q, err := b.Querier(start, end)
	if err != nil {
		return nil, err
	}
	defer q.Close()
	set := q.Select(ctx, true, nil, ms...)
	out := []RawSeries{}
	count := 0
	for set.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(out) >= 10000 {
			return nil, errors.New("selection exceeds 10000 series; narrow selectors")
		}
		series := set.At()
		name, ls := seriesIdentity(series.Labels())
		row := RawSeries{Name: name, Labels: ls, Points: []model.Point{}}
		if values {
			it := series.Iterator(nil)
			for kind := it.Next(); kind != chunkenc.ValNone; kind = it.Next() {
				if kind != chunkenc.ValFloat {
					return nil, errors.New("unsupported non-float observation")
				}
				at, value := it.At()
				count++
				if count > 1000000 {
					return nil, errors.New("raw selection exceeds 1000000 samples; narrow selectors")
				}
				row.Points = append(row.Points, model.Point{Timestamp: at, Value: value})
			}
			if err := it.Err(); err != nil {
				return nil, err
			}
			if len(row.Points) == 0 {
				continue
			}
		} else {
			it := series.Iterator(nil)
			if it.Next() == chunkenc.ValNone {
				if err := it.Err(); err != nil {
					return nil, err
				}
				continue
			}
		}
		out = append(out, row)
	}
	return out, set.Err()
}
func (b *Embedded) LoadSeries(ctx context.Context, start, end int64, ms []Matcher) ([]RawSeries, error) {
	return b.readSeries(ctx, start, end, ms, true)
}
func (b *Embedded) SelectSeries(ctx context.Context, start, end int64, ms []Matcher) ([]RawSeries, error) {
	return b.readSeries(ctx, start, end, ms, false)
}

func (b *Embedded) Query(ctx context.Context, q model.Query) (model.QueryResult, error) {
	out := model.QueryResult{Metric: q.Metric, Aggregation: q.Aggregation, Start: q.Start, End: q.End, Step: q.Step, Series: []model.Series{}}
	if q.Start < 0 || q.End <= q.Start || q.End-q.Start > int64(31*24*time.Hour/time.Millisecond) || q.Step < 1000 || (q.End-q.Start)/q.Step > 2000 || q.Metric == "" || !model.ValidAggregation(q.Aggregation) {
		return out, errors.New("invalid metric query range, step or aggregation")
	}
	if err := model.ValidateLabels(q.Labels); err != nil {
		return out, err
	}
	ms := []Matcher{{Name: labels.MetricName, Type: "=", Value: q.Metric}}
	for k, v := range q.Labels {
		ms = append(ms, Matcher{Name: k, Type: "=", Value: v})
	}
	matchers, err := embeddedMatchers(ms)
	if err != nil {
		return out, err
	}
	from := q.Start
	if q.Aggregation == "rate" {
		from -= q.Step
	}
	reader, err := b.Querier(from, q.End)
	if err != nil {
		return out, err
	}
	defer reader.Close()
	set := reader.Select(ctx, true, nil, matchers...)
	states := map[int64]*bucketState{}
	ids := []int64{}
	count := 0
	for set.Next() {
		if len(ids) >= maxNativeSeries {
			return out, errors.New("query exceeds 200 series; add label filters")
		}
		series := set.At()
		_, ls := seriesIdentity(series.Labels())
		state := &bucketState{labels: ls, buckets: map[int64]float64{}}
		id := int64(len(ids))
		states[id] = state
		ids = append(ids, id)
		it := series.Iterator(nil)
		for kind := it.Next(); kind != chunkenc.ValNone; kind = it.Next() {
			count++
			limit := maxSQLiteBucketSamples
			if q.Aggregation == "rate" {
				limit = maxSQLiteRateSamples
			}
			if count > limit {
				return out, errors.New("query sample budget exceeded; narrow selectors")
			}
			if count%1024 == 0 {
				if err := ctx.Err(); err != nil {
					return out, err
				}
			}
			if kind != chunkenc.ValFloat {
				return out, errors.New("unsupported non-float observation")
			}
			at, value := it.At()
			if at >= q.Start {
				bucket := q.Start + (at-q.Start)/q.Step*q.Step
				if q.Aggregation == "rate" {
					if state.previous != nil && at > state.previous.Timestamp {
						delta := value - state.previous.Value
						if delta < 0 {
							delta = value
						}
						rate := delta / (float64(at-state.previous.Timestamp) / 1000)
						if !math.IsNaN(rate) && !math.IsInf(rate, 0) {
							state.buckets[bucket] = rate
						}
					}
				} else {
					state.buckets[bucket] = value
				}
			}
			state.previous = &model.Point{Timestamp: at, Value: value}
		}
		if err := it.Err(); err != nil {
			return out, err
		}
	}
	if err := set.Err(); err != nil {
		return out, err
	}
	return finishBucketQuery(out, q, states, ids)
}

func (b *Embedded) Metrics(ctx context.Context) ([]Metric, error) {
	q, err := b.Querier(0, time.Now().Add(5*time.Minute).UnixMilli())
	if err != nil {
		return nil, err
	}
	defer q.Close()
	set := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, ""))
	counts := map[string]int{}
	total := 0
	for set.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		series := set.At()
		it := series.Iterator(nil)
		if it.Next() == chunkenc.ValNone {
			if err := it.Err(); err != nil {
				return nil, err
			}
			continue
		}
		total++
		if total > EmbeddedMaxSeries {
			return nil, errors.New("metric catalog exceeds the series budget")
		}
		counts[series.Labels().Get(labels.MetricName)]++
	}
	if err := set.Err(); err != nil {
		return nil, err
	}
	out := []Metric{}
	for name, count := range counts {
		out = append(out, Metric{Name: name, SeriesCount: count})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (b *Embedded) Stats(ctx context.Context) (Stats, error) {
	b.life.RLock()
	defer b.life.RUnlock()
	if b.closed.Load() {
		return Stats{}, errors.New("embedded storage is closed")
	}
	b.statsMu.Lock()
	defer b.statsMu.Unlock()
	if time.Since(b.statsAt) > 30*time.Second {
		start, end := time.Now().Add(-time.Duration(b.days.Load())*24*time.Hour).UnixMilli(), time.Now().Add(5*time.Minute).UnixMilli()
		q, err := b.db.ChunkQuerier(start, end)
		if err != nil {
			return Stats{}, err
		}
		set := q.Select(ctx, false, nil, labels.MustNewMatcher(labels.MatchNotEqual, labels.MetricName, ""))
		v := Stats{Backend: "embedded"}
		for set.Next() {
			if err := ctx.Err(); err != nil {
				q.Close()
				return Stats{}, err
			}
			if v.Series >= EmbeddedMaxSeries {
				q.Close()
				return Stats{}, errors.New("statistics exceed the series budget")
			}
			it := set.At().Iterator(nil)
			n := 0
			for it.Next() {
				chunk := it.At()
				if chunk.Chunk != nil {
					if chunk.MinTime >= start && chunk.MaxTime <= end {
						n += chunk.Chunk.NumSamples()
					} else {
						points := chunk.Chunk.Iterator(nil)
						for kind := points.Seek(start); kind != chunkenc.ValNone; kind = points.Next() {
							if points.AtT() > end {
								break
							}
							n++
						}
						if err := points.Err(); err != nil {
							q.Close()
							return Stats{}, err
						}
					}
				}
			}
			if err := it.Err(); err != nil {
				q.Close()
				return Stats{}, err
			}
			if n > 0 {
				v.Series++
				v.Samples += n
			}
		}
		err = set.Err()
		q.Close()
		if err != nil {
			return Stats{}, err
		}
		err = filepath.WalkDir(b.path, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err == nil && !d.IsDir() {
				if info, e := d.Info(); e == nil {
					size := allocatedFileBytes(info)
					v.StorageBytes += size
					rel, _ := filepath.Rel(b.path, path)
					if strings.HasPrefix(rel, "timeseries"+string(filepath.Separator)) {
						v.TimeSeriesBytes += size
					}
					if strings.HasPrefix(rel, "vectors"+string(filepath.Separator)) {
						v.VectorBytes += size
					}
				} else {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return Stats{}, err
		}
		for _, path := range []string{b.controlPath, b.controlPath + "-wal", b.controlPath + "-shm"} {
			if info, e := os.Stat(path); e == nil {
				size := allocatedFileBytes(info)
				v.ControlBytes += size
				v.StorageBytes += size
			}
		}
		v.Vectors = b.vectors.Count()
		b.stats, b.statsAt = v, time.Now()
	}
	out := b.stats
	out.LastSample = b.lastSample.Load()
	out.RetentionDays = int(b.days.Load())
	b.rateMu.Lock()
	sec := time.Now().Unix()
	for _, slot := range b.rate {
		if slot.at > sec-60 {
			out.IngestRate += float64(slot.count) / 60
		}
	}
	b.rateMu.Unlock()
	return out, nil
}
