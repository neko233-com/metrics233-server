package vectorstore

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func fixture(i int) model.Pattern {
	values := make([]float32, 64)
	for n := range values {
		values[n] = float32(math.Sin(float64(n)/9+float64(i)/20) + float64(i)/100)
	}
	return model.Pattern{Metric: "game_latency", Labels: map[string]string{"serverId": string(rune('a' + i%3))}, Start: 1000, End: 65000, Aggregation: "last", Normalization: "shape", Values: values, Coverage: 1, CreatedAt: 100000 + int64(i)}
}

func TestVectorDatabasePersistsFiltersDeletionAndCapacity(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "vectors")
	s, err := Open(path, 10)
	require.NoError(t, err)
	batch := []model.Pattern{}
	for i := 0; i < 10; i++ {
		batch = append(batch, fixture(i))
	}
	require.NoError(t, s.Save(ctx, batch))
	require.Equal(t, 10, s.Count())
	require.NoError(t, s.Save(ctx, batch))
	require.Equal(t, 10, s.Count(), "immutable IDs deduplicate")
	for i := range batch {
		batch[i].SetID()
	}
	reference := batch[3]
	hits, err := s.Search(ctx, model.PatternSearch{Reference: reference, Limit: 3, Labels: map[string]string{"serverId": "a"}, Exact: true})
	require.NoError(t, err)
	require.Len(t, hits, 3)
	for _, hit := range hits {
		require.Equal(t, "a", hit.Pattern.Labels["serverId"])
		require.NotEqual(t, reference.ID, hit.Pattern.ID)
	}
	_, err = s.Get(ctx, reference.ID)
	require.NoError(t, err)
	require.Error(t, s.Save(ctx, []model.Pattern{fixture(20)}), "capacity fails before partial commit")
	require.NoError(t, s.Close())
	s, err = Open(path, 10)
	require.NoError(t, err)
	defer s.Close()
	loaded, err := s.Get(ctx, reference.ID)
	require.NoError(t, err)
	require.Equal(t, reference.Values, loaded.Values)
	require.NoError(t, s.Delete(ctx, reference.ID))
	_, err = s.Get(ctx, reference.ID)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Equal(t, 9, s.Count())
	require.NoError(t, s.Prune(ctx, 65001))
	require.Zero(t, s.Count())
	listed, err := s.List(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, listed)
}

func TestVectorDatabaseANNRecallAndSparseFilterFallback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "vectors"), 2000)
	require.NoError(t, err)
	defer s.Close()
	all := []model.Pattern{}
	for i := 0; i < 1000; i++ {
		p := fixture(i)
		if i == 7 {
			p.Labels["rare"] = "yes"
		}
		all = append(all, p)
	}
	for i := 0; i < len(all); i += 100 {
		require.NoError(t, s.Save(t.Context(), all[i:i+100]))
	}
	ref := fixture(500)
	ref.Values[0] += .01
	ref.SetID()
	exact, err := s.Search(t.Context(), model.PatternSearch{Reference: ref, Limit: 10, Exact: true})
	require.NoError(t, err)
	ann, err := s.Search(t.Context(), model.PatternSearch{Reference: ref, Limit: 10})
	require.NoError(t, err)
	require.Len(t, ann, 10)
	set := map[string]bool{}
	for _, hit := range exact {
		set[hit.Pattern.ID] = true
	}
	matches := 0
	for _, hit := range ann {
		if set[hit.Pattern.ID] {
			matches++
		}
	}
	require.GreaterOrEqual(t, matches, 9, "ANN recall must remain useful on the deterministic fixture")
	rare, err := s.Search(t.Context(), model.PatternSearch{Reference: ref, Limit: 10, Labels: map[string]string{"rare": "yes"}})
	require.NoError(t, err)
	require.Len(t, rare, 1)
	q := model.PatternSearch{Reference: ref, Limit: 10}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = s.Search(ctx, q)
	require.True(t, errors.Is(err, context.Canceled))
}

func TestVectorDatabaseZeroVectorsAndNormalizationIsolation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "vectors"), 10)
	require.NoError(t, err)
	defer s.Close()
	p := fixture(0)
	p.Values = make([]float32, 64)
	other := p
	other.Normalization = "raw"
	require.NoError(t, s.Save(t.Context(), []model.Pattern{p, other}))
	p.SetID()
	hits, err := s.Search(t.Context(), model.PatternSearch{Reference: p, Limit: 10, IncludeSelf: true})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	require.Zero(t, hits[0].Distance)
	start := time.Now()
	listed, err := s.List(t.Context(), 10)
	require.NoError(t, err)
	require.Len(t, listed, 2)
	t.Logf("empty-scale vector database disk=%d bytes list=%s", s.DiskBytes(), time.Since(start))
}
