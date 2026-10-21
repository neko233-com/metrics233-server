package server_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"math"
	"net/http/httptest"
	"testing"
	"time"

	promproto "github.com/gogo/protobuf/proto"
	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	promvalue "github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRemoteWriteRejectsDeclaredDecodedSizeBeforeAllocation(t *testing.T) {
	_, handler := setup(t, "")
	// Snappy's size prefix declares a huge output without allocating that output
	// or supplying a valid compressed stream. Size rejection must precede Decode.
	header := make([]byte, binary.MaxVarintLen64)
	length := binary.PutUvarint(header, (64<<20)+1)
	request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(header[:length]))
	request.Header.Set("Content-Encoding", "snappy")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, 413, response.Code, response.Body.String())
	assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
}

func TestRemoteWriteRejectsV2NativeHistogramDuplicatesAndLateInvalidSampleWithoutWrites(t *testing.T) {
	for _, check := range []struct {
		name        string
		contentType string
		mutate      func(*prompb.WriteRequest)
		status      int
	}{
		{"v2", "application/x-protobuf;proto=io.prometheus.write.v2.Request", func(*prompb.WriteRequest) {}, 415},
		{"native-histogram", "", func(r *prompb.WriteRequest) { r.Timeseries[0].Histograms = []prompb.Histogram{{Schema: 0}} }, 400},
		{"stale-marker", "", func(r *prompb.WriteRequest) {
			r.Timeseries[0].Samples[0].Value = math.Float64frombits(promvalue.StaleNaN)
		}, 400},
		{"duplicate-label", "", func(r *prompb.WriteRequest) {
			r.Timeseries[0].Labels = append(r.Timeseries[0].Labels, prompb.Label{Name: "job", Value: "duplicate"})
		}, 400},
		{"late-invalid", "", func(r *prompb.WriteRequest) {
			r.Timeseries[0].Samples = make([]prompb.Sample, 10001)
			for index := range r.Timeseries[0].Samples {
				r.Timeseries[0].Samples[index] = prompb.Sample{Value: 1, Timestamp: time.Now().UnixMilli()}
			}
			r.Timeseries[0].Samples[10000].Value = math.NaN()
		}, 400},
		{"sample-cap", "", func(r *prompb.WriteRequest) { r.Timeseries[0].Samples = make([]prompb.Sample, 100001) }, 413},
	} {
		t.Run(check.name, func(t *testing.T) {
			db, handler := setup(t, "")
			fixture := remoteWriteFixture(time.Now().UnixMilli())
			check.mutate(fixture)
			body, err := promproto.Marshal(fixture)
			require.NoError(t, err)
			request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
			request.Header.Set("Content-Type", check.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, check.status, response.Code, response.Body.String())
			assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
			if check.name == "stale-marker" {
				assert.Contains(t, response.Body.String(), "staleness storage")
			}
			stored, err := db.Metrics(context.Background())
			require.NoError(t, err)
			assert.Empty(t, stored)
		})
	}
}

type interruptedIngestBackend struct {
	store.MetricBackend
	calls, accepted, failAt int
}

func (backend *interruptedIngestBackend) Ingest(_ context.Context, samples []model.Sample) error {
	backend.calls++
	if backend.calls == backend.failAt {
		return errors.New("simulated storage outage")
	}
	backend.accepted += len(samples)
	return nil
}

func TestRemoteWriteRequiresMetricNameBeforeAnyBatch(t *testing.T) {
	for _, labels := range [][]prompb.Label{
		{{Name: "job", Value: "missing-name"}},
		{{Name: "__name__", Value: ""}},
	} {
		t.Run(labels[0].Name, func(t *testing.T) {
			db, handler := setup(t, "")
			backend := &interruptedIngestBackend{}
			db.Backend = backend
			fixture := remoteWriteFixture(time.Now().UnixMilli())
			point := fixture.Timeseries[0].Samples[0]
			fixture.Timeseries[0].Samples = make([]prompb.Sample, 10001)
			for index := range fixture.Timeseries[0].Samples {
				fixture.Timeseries[0].Samples[index] = point
			}
			fixture.Timeseries = append(fixture.Timeseries, prompb.TimeSeries{Labels: labels, Samples: []prompb.Sample{point}})
			body, err := promproto.Marshal(fixture)
			require.NoError(t, err)
			request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, 400, response.Code, response.Body.String())
			assert.Contains(t, response.Body.String(), "metric name")
			assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
			assert.Zero(t, backend.calls, "a nameless sample must reject before committing a valid prefix")
		})
	}
}

func TestRemoteWriteAcceptsEmptyAndMetadataOnlyRequests(t *testing.T) {
	db, handler := setup(t, "")
	backend := &interruptedIngestBackend{}
	db.Backend = backend
	for _, fixture := range []*prompb.WriteRequest{
		{},
		{Metadata: []prompb.MetricMetadata{{MetricFamilyName: "metadata_only", Type: prompb.MetricMetadata_GAUGE}}},
		{Timeseries: []prompb.TimeSeries{{Labels: []prompb.Label{{Name: "job", Value: "empty-series"}}}, {Labels: []prompb.Label{{Name: "__name__", Value: ""}}}}},
	} {
		body, err := promproto.Marshal(fixture)
		require.NoError(t, err)
		request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, 204, response.Code, response.Body.String())
		assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
	}
	assert.Zero(t, backend.calls)
}

func TestRemoteWriteStorageFailureIsRetryableAndReportsCompletedBatches(t *testing.T) {
	db, handler := setup(t, "")
	backend := &interruptedIngestBackend{failAt: 2}
	db.Backend = backend
	fixture := remoteWriteFixture(time.Now().UnixMilli())
	point := fixture.Timeseries[0].Samples[0]
	fixture.Timeseries[0].Samples = make([]prompb.Sample, 10001)
	for index := range fixture.Timeseries[0].Samples {
		fixture.Timeseries[0].Samples[index] = point
	}
	body, err := promproto.Marshal(fixture)
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/api/v1/write", bytes.NewReader(body))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	assert.Equal(t, 503, response.Code, response.Body.String())
	assert.Equal(t, "10000", response.Header().Get("X-Accepted-Samples"))
	assert.Equal(t, 10000, backend.accepted)
}
