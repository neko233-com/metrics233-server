// Package vectorstore owns a local, durable 64-dimensional window database.
// Payloads and secondary indexes live in a compressed LSM; small HNSW graphs
// accelerate approximate search. It has no listener or dependency on SQL.
package vectorstore

import (
	"container/heap"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/dgraph-io/badger/v4/options"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/vectorstore/hnsw"
)

const DefaultMaxVectors = 100000

type Store struct {
	mu           sync.RWMutex
	db           *badger.DB
	sequence     *badger.Sequence
	graphs       map[string]*hnsw.Graph[uint64]
	count, limit int
	path         string
	closed       bool
}

func Open(path string, limit int) (*Store, error) {
	if limit <= 0 {
		limit = DefaultMaxVectors
	}
	if limit > DefaultMaxVectors {
		return nil, errors.New("embedded vector capacity exceeds the supported budget")
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	opts := badger.DefaultOptions(path).WithLogger(nil).WithSyncWrites(true).WithCompression(options.ZSTD).
		WithZSTDCompressionLevel(3).WithMemTableSize(4 << 20).WithNumMemtables(2).WithBaseTableSize(1 << 20).
		WithBaseLevelSize(8 << 20).WithBlockCacheSize(4 << 20).WithIndexCacheSize(2 << 20).WithValueLogFileSize(2 << 20).WithValueThreshold(64 << 10)
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open vector database: %w", err)
	}
	s := &Store{db: db, graphs: map[string]*hnsw.Graph[uint64]{}, limit: limit, path: path}
	s.sequence, err = db.GetSequence([]byte("sequence"), 1000)
	if err != nil {
		db.Close()
		return nil, err
	}
	err = db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte{'P'}
		it := tx.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			key := it.Item().KeyCopy(nil)
			if len(key) != 9 {
				return errors.New("invalid vector record key")
			}
			p, err := readPattern(it.Item())
			if err != nil {
				return err
			}
			s.count++
			if s.count > s.limit {
				return errors.New("vector database exceeds configured memory budget")
			}
			s.graph(p).Add(hnsw.MakeNode(binary.BigEndian.Uint64(key[1:]), p.Values))
		}
		return nil
	})
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) graph(p model.Pattern) *hnsw.Graph[uint64] {
	key := p.Normalization + "/" + p.Aggregation
	g := s.graphs[key]
	if g == nil {
		g = hnsw.NewGraph[uint64]()
		g.Distance = hnsw.EuclideanDistance
		g.M = 12
		g.EfSearch = 80
		s.graphs[key] = g
	}
	return g
}

func numberKey(prefix byte, id uint64) []byte {
	b := make([]byte, 9)
	b[0] = prefix
	binary.BigEndian.PutUint64(b[1:], id)
	return b
}
func identityKey(id string) ([]byte, error) {
	raw, err := hex.DecodeString(id)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid vector identity")
	}
	return append([]byte{'I'}, raw...), nil
}
func collectionPrefix(p model.Pattern, metric string) []byte {
	n, a := byte(1), byte(1)
	if p.Normalization == "raw" {
		n = 2
	}
	if p.Aggregation == "rate" {
		a = 2
	}
	if metric == "" {
		return []byte{'N', n, a}
	}
	return append(append([]byte{'M', n, a}, []byte(metric)...), 0)
}
func indexKeys(p model.Pattern, id uint64) [][]byte {
	latest := make([]byte, 17)
	latest[0] = 'L'
	binary.BigEndian.PutUint64(latest[1:], ^uint64(p.CreatedAt))
	binary.BigEndian.PutUint64(latest[9:], id)
	return [][]byte{latest, append(collectionPrefix(p, ""), numberKey(0, id)[1:]...), append(collectionPrefix(p, p.Metric), numberKey(0, id)[1:]...)}
}

func encodePattern(p model.Pattern) ([]byte, error) {
	metadata, err := json.Marshal(p.Summary())
	if err != nil || len(metadata) > 128<<10 {
		return nil, errors.New("vector metadata exceeds budget")
	}
	out := make([]byte, 4+len(metadata)+model.PatternDimensions*4)
	binary.BigEndian.PutUint32(out, uint32(len(metadata)))
	copy(out[4:], metadata)
	for i, value := range p.Values {
		binary.BigEndian.PutUint32(out[4+len(metadata)+i*4:], math.Float32bits(value))
	}
	return out, nil
}
func readPattern(item *badger.Item) (model.Pattern, error) {
	var p model.Pattern
	err := item.Value(func(raw []byte) error {
		if len(raw) < 4 {
			return errors.New("invalid vector record")
		}
		n := int(binary.BigEndian.Uint32(raw))
		if n > 128<<10 || len(raw) != 4+n+model.PatternDimensions*4 {
			return errors.New("invalid vector payload size")
		}
		if err := json.Unmarshal(raw[4:4+n], &p); err != nil {
			return err
		}
		p.Values = make([]float32, model.PatternDimensions)
		for i := range p.Values {
			p.Values[i] = math.Float32frombits(binary.BigEndian.Uint32(raw[4+n+i*4:]))
		}
		return p.Validate()
	})
	return p, err
}

func (s *Store) Save(ctx context.Context, patterns []model.Pattern) error {
	if len(patterns) < 1 || len(patterns) > 200 {
		return errors.New("vector batch must contain 1–200 windows")
	}
	for i := range patterns {
		if err := patterns[i].Validate(); err != nil {
			return err
		}
		patterns[i].SetID()
		if patterns[i].CreatedAt == 0 {
			patterns[i].CreatedAt = time.Now().UnixMilli()
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("vector database is closed")
	}
	type addition struct {
		p  model.Pattern
		id uint64
	}
	added := []addition{}
	err := s.db.Update(func(tx *badger.Txn) error {
		for _, p := range patterns {
			if err := ctx.Err(); err != nil {
				return err
			}
			key, _ := identityKey(p.ID)
			if _, err := tx.Get(key); err == nil {
				continue
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}
			if s.count+len(added) >= s.limit {
				return fmt.Errorf("vector capacity %d reached; narrow scheduled windows or prune expired data", s.limit)
			}
			id, err := s.sequence.Next()
			if err != nil {
				return err
			}
			raw, err := encodePattern(p)
			if err != nil {
				return err
			}
			if err = tx.Set(numberKey('P', id), raw); err != nil {
				return err
			}
			if err = tx.Set(key, numberKey(0, id)[1:]); err != nil {
				return err
			}
			for _, index := range indexKeys(p, id) {
				if err = tx.Set(index, nil); err != nil {
					return err
				}
			}
			added = append(added, addition{p, id})
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, a := range added {
		s.graph(a.p).Add(hnsw.MakeNode(a.id, append([]float32(nil), a.p.Values...)))
	}
	s.count += len(added)
	return nil
}

func patternByID(tx *badger.Txn, id string) (model.Pattern, uint64, error) {
	key, err := identityKey(id)
	if err != nil {
		return model.Pattern{}, 0, sql.ErrNoRows
	}
	item, err := tx.Get(key)
	if errors.Is(err, badger.ErrKeyNotFound) {
		return model.Pattern{}, 0, sql.ErrNoRows
	}
	if err != nil {
		return model.Pattern{}, 0, err
	}
	var number uint64
	err = item.Value(func(raw []byte) error {
		if len(raw) != 8 {
			return errors.New("invalid vector identity record")
		}
		number = binary.BigEndian.Uint64(raw)
		return nil
	})
	if err != nil {
		return model.Pattern{}, 0, err
	}
	item, err = tx.Get(numberKey('P', number))
	if err != nil {
		return model.Pattern{}, 0, err
	}
	p, err := readPattern(item)
	return p, number, err
}

func (s *Store) Get(ctx context.Context, id string) (model.Pattern, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var p model.Pattern
	if s.closed {
		return p, errors.New("vector database is closed")
	}
	err := s.db.View(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		p, _, err = patternByID(tx, id)
		return err
	})
	return p, err
}
func (s *Store) List(ctx context.Context, limit int) ([]model.Pattern, error) {
	if limit < 1 || limit > 1000 {
		return nil, errors.New("limit must be 1–1000")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("vector database is closed")
	}
	out := []model.Pattern{}
	err := s.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte{'L'}
		opts.PrefetchValues = false
		it := tx.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid() && len(out) < limit; it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			key := it.Item().KeyCopy(nil)
			if len(key) != 17 {
				return errors.New("invalid vector time index")
			}
			item, err := tx.Get(numberKey('P', binary.BigEndian.Uint64(key[9:])))
			if err != nil {
				return err
			}
			p, err := readPattern(item)
			if err != nil {
				return err
			}
			out = append(out, p.Summary())
		}
		return nil
	})
	return out, err
}

func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("vector database is closed")
	}
	var p model.Pattern
	var number uint64
	err := s.db.Update(func(tx *badger.Txn) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var err error
		p, number, err = patternByID(tx, id)
		if err != nil {
			return err
		}
		return deleteRecord(tx, p, number)
	})
	if err != nil {
		return err
	}
	s.graph(p).Delete(number)
	s.count--
	return nil
}
func deleteRecord(tx *badger.Txn, p model.Pattern, id uint64) error {
	identity, _ := identityKey(p.ID)
	keys := append(indexKeys(p, id), identity, numberKey('P', id))
	for _, key := range keys {
		if err := tx.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Prune(ctx context.Context, before int64) error {
	// Read bounded IDs before mutation; ordinary Delete keeps graph/LSM atomic
	// under the same lock. A canceled cleanup never returns unbounded stale hits.
	ids := []string{}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return errors.New("vector database is closed")
	}
	err := s.db.View(func(tx *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = []byte{'P'}
		it := tx.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			if err := ctx.Err(); err != nil {
				return err
			}
			p, err := readPattern(it.Item())
			if err != nil {
				return err
			}
			if p.End < before {
				ids = append(ids, p.ID)
			}
		}
		return nil
	})
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.Delete(ctx, id); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
	}
	// Small payloads live in compressed SSTs; reclaim overwritten value-log data
	// opportunistically without turning routine retention into a blocking loop.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.closed {
		_ = s.db.RunValueLogGC(.5)
	}
	return nil
}

func (s *Store) Search(ctx context.Context, q model.PatternSearch) ([]model.PatternHit, error) {
	if err := q.Reference.Validate(); err != nil {
		return nil, err
	}
	if q.Limit < 1 || q.Limit > 100 {
		return nil, errors.New("limit must be 1–100")
	}
	if err := model.ValidateLabels(q.Labels); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("vector database is closed")
	}
	top := &hitHeap{}
	heap.Init(top)
	seen := map[uint64]bool{}
	err := s.db.View(func(tx *badger.Txn) error {
		consider := func(id uint64) error {
			if seen[id] {
				return nil
			}
			seen[id] = true
			if err := ctx.Err(); err != nil {
				return err
			}
			item, err := tx.Get(numberKey('P', id))
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			p, err := readPattern(item)
			if err != nil {
				return err
			}
			if p.Normalization != q.Reference.Normalization || p.Aggregation != q.Reference.Aggregation || q.Metric != "" && p.Metric != q.Metric || !q.IncludeSelf && p.ID == q.Reference.ID {
				return nil
			}
			for k, v := range q.Labels {
				if p.Labels[k] != v {
					return nil
				}
			}
			d := 0.0
			for i, value := range p.Values {
				delta := float64(value) - float64(q.Reference.Values[i])
				d += delta * delta
			}
			hit := model.PatternHit{Pattern: p.Summary(), Distance: math.Sqrt(d)}
			if top.Len() < q.Limit {
				heap.Push(top, hit)
			} else if closer(hit, (*top)[0]) {
				(*top)[0] = hit
				heap.Fix(top, 0)
			}
			return nil
		}
		if !q.Exact {
			if g := s.graphs[q.Reference.Normalization+"/"+q.Reference.Aggregation]; g != nil {
				for _, node := range g.Search(q.Reference.Values, min(s.count, max(80, q.Limit*8))) {
					if err := consider(node.Key); err != nil {
						return err
					}
				}
			}
		}
		// Sparse filters cannot disappear behind ANN post-filtering. Exact mode or
		// an underfilled filtered result scans the collection/metric secondary index.
		if q.Exact || top.Len() < q.Limit {
			opts := badger.DefaultIteratorOptions
			opts.Prefix = collectionPrefix(q.Reference, q.Metric)
			opts.PrefetchValues = false
			it := tx.NewIterator(opts)
			defer it.Close()
			for it.Rewind(); it.Valid(); it.Next() {
				key := it.Item().KeyCopy(nil)
				if len(key) < 8 {
					return errors.New("invalid vector collection index")
				}
				if err := consider(binary.BigEndian.Uint64(key[len(key)-8:])); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := []model.PatternHit(*top)
	sort.Slice(out, func(i, j int) bool { return closer(out[i], out[j]) })
	return out, nil
}

func (s *Store) Count() int { s.mu.RLock(); defer s.mu.RUnlock(); return s.count }
func (s *Store) DiskBytes() int64 {
	var total int64
	_ = filepath.WalkDir(s.path, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, e := d.Info(); e == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return errors.Join(s.sequence.Release(), s.db.Close())
}

func closer(a, b model.PatternHit) bool {
	if a.Distance == b.Distance {
		return a.Pattern.ID < b.Pattern.ID
	}
	return a.Distance < b.Distance
}

type hitHeap []model.PatternHit

func (h hitHeap) Len() int           { return len(h) }
func (h hitHeap) Less(i, j int) bool { return closer(h[j], h[i]) }
func (h hitHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *hitHeap) Push(value any)    { *h = append(*h, value.(model.PatternHit)) }
func (h *hitHeap) Pop() any          { old := *h; last := old[len(old)-1]; *h = old[:len(old)-1]; return last }
