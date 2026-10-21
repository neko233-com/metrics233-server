package server_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func exportOTLP(t *testing.T, handler http.Handler, metrics []*metricspb.Metric, jsonRequest bool) (*httptest.ResponseRecorder, *collectormetrics.ExportMetricsServiceResponse) {
	t.Helper()
	payload := &collectormetrics.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: metrics}}}}}
	body, err := proto.Marshal(payload)
	contentType := "application/x-protobuf"
	if jsonRequest {
		body, err = protojson.Marshal(payload)
		contentType = "application/json"
	}
	require.NoError(t, err)
	request := httptest.NewRequest("POST", "/otlp/v1/metrics", bytes.NewReader(body))
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	var result collectormetrics.ExportMetricsServiceResponse
	if response.Code == 200 {
		if jsonRequest {
			require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), &result))
		} else {
			require.NoError(t, proto.Unmarshal(response.Body.Bytes(), &result))
		}
	}
	return response, &result
}

func otlpGauge(name string, points ...*metricspb.NumberDataPoint) *metricspb.Metric {
	return &metricspb.Metric{Name: name, Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: points}}}
}

func TestOTLPPartialSuccessCountsOriginalPointsAndPreservesValidZero(t *testing.T) {
	for _, jsonRequest := range []bool{false, true} {
		t.Run(map[bool]string{false: "protobuf", true: "json"}[jsonRequest], func(t *testing.T) {
			db, handler := setup(t, "")
			metrics := []*metricspb.Metric{
				otlpGauge("valid_zero", &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 0}}),
				otlpGauge("bad_value", &metricspb.NumberDataPoint{}, &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: math.Inf(1)}}, &metricspb.NumberDataPoint{Flags: 1, Value: &metricspb.NumberDataPoint_AsInt{AsInt: 42}}),
				{Name: "delta", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA, DataPoints: []*metricspb.NumberDataPoint{{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 5}}}}}},
				{Name: "exponential", Data: &metricspb.Metric_ExponentialHistogram{ExponentialHistogram: &metricspb.ExponentialHistogram{DataPoints: []*metricspb.ExponentialHistogramDataPoint{{Count: 3}}}}},
			}
			response, result := exportOTLP(t, handler, metrics, jsonRequest)
			require.Equal(t, 200, response.Code, response.Body.String())
			assert.Equal(t, "1", response.Header().Get("X-Accepted-Samples"))
			assert.Equal(t, int64(5), result.GetPartialSuccess().GetRejectedDataPoints())
			assert.NotEmpty(t, result.GetPartialSuccess().GetErrorMessage())
			stored, err := db.Metrics(context.Background())
			require.NoError(t, err)
			require.Len(t, stored, 1)
			assert.Equal(t, "valid_zero", stored[0].Name)
		})
	}
}

func TestOTLPHistogramAndSummaryIdentityAndOptionalSum(t *testing.T) {
	db, handler := setup(t, "")
	label := func(name string) []*commonpb.KeyValue {
		return []*commonpb.KeyValue{{Key: name, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "must-not-overwrite-protocol-label"}}}}
	}
	metrics := []*metricspb.Metric{
		{Name: "cumulative_counter", Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, IsMonotonic: true, DataPoints: []*metricspb.NumberDataPoint{{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 23}}}}}},
		{Name: "duration", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{{Attributes: label("le"), Count: 3, BucketCounts: []uint64{2, 1}, ExplicitBounds: []float64{0.1}}}}}},
		{Name: "distribution_omitted", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{{Count: 3, Sum: proto.Float64(0.25)}}}}},
		{Name: "summary", Data: &metricspb.Metric_Summary{Summary: &metricspb.Summary{DataPoints: []*metricspb.SummaryDataPoint{{Attributes: label("quantile"), Count: 3, Sum: 0.25, QuantileValues: []*metricspb.SummaryDataPoint_ValueAtQuantile{{Quantile: 0.5, Value: 0.1}}}}}}},
	}
	response, result := exportOTLP(t, handler, metrics, false)
	require.Equal(t, 200, response.Code, response.Body.String())
	assert.Nil(t, result.PartialSuccess)
	assert.Equal(t, "9", response.Header().Get("X-Accepted-Samples"))
	metricsStored, err := db.Metrics(context.Background())
	require.NoError(t, err)
	for _, metric := range metricsStored {
		assert.NotEqual(t, "duration_sum", metric.Name, "absent OTLP sum must not fabricate zero")
	}
	for _, check := range []struct {
		name   string
		labels map[string]string
		want   float64
	}{
		{"cumulative_counter", nil, 23},
		{"duration_bucket", map[string]string{"le": "+Inf"}, 3},
		{"duration_bucket", map[string]string{"le": "0.1"}, 2},
		{"summary", map[string]string{"quantile": "0.5"}, 0.1},
	} {
		now := time.Now().UnixMilli()
		query, err := db.Query(context.Background(), model.Query{Metric: check.name, Start: now - 60000, End: now + 1000, Step: 1000, Aggregation: "last", Labels: check.labels})
		require.NoError(t, err)
		require.Len(t, query.Series, 1)
		assert.Equal(t, check.want, query.Series[0].Points[0].Value)
	}
}

func TestOTLPRejectsInvalidHistogramAsOneWholePoint(t *testing.T) {
	for _, change := range []struct {
		name   string
		mutate func(*metricspb.HistogramDataPoint)
	}{
		{"length", func(p *metricspb.HistogramDataPoint) { p.BucketCounts = []uint64{3} }},
		{"count", func(p *metricspb.HistogramDataPoint) { p.Count = 4 }},
		{"unsorted", func(p *metricspb.HistogramDataPoint) {
			p.ExplicitBounds = []float64{2, 1}
			p.BucketCounts = []uint64{1, 1, 1}
		}},
		{"nonfinite", func(p *metricspb.HistogramDataPoint) { p.ExplicitBounds = []float64{math.Inf(1)} }},
		{"overflow", func(p *metricspb.HistogramDataPoint) {
			p.Count = math.MaxUint64
			p.BucketCounts = []uint64{math.MaxUint64, 1}
		}},
		{"empty-sum", func(p *metricspb.HistogramDataPoint) {
			p.Count = 0
			p.BucketCounts = []uint64{0, 0}
			p.Sum = proto.Float64(1)
		}},
	} {
		t.Run(change.name, func(t *testing.T) {
			db, handler := setup(t, "")
			point := &metricspb.HistogramDataPoint{Count: 3, BucketCounts: []uint64{2, 1}, ExplicitBounds: []float64{0.1}}
			change.mutate(point)
			response, result := exportOTLP(t, handler, []*metricspb.Metric{{Name: "invalid_histogram", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{point}}}}}, false)
			require.Equal(t, 200, response.Code, response.Body.String())
			assert.Equal(t, int64(1), result.GetPartialSuccess().GetRejectedDataPoints())
			assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
			stored, err := db.Metrics(context.Background())
			require.NoError(t, err)
			assert.Empty(t, stored)
		})
	}
}

func TestOTLPTransportErrorsAndGzip(t *testing.T) {
	_, handler := setup(t, "")
	compress := func(body []byte) []byte {
		var buffer bytes.Buffer
		writer := gzip.NewWriter(&buffer)
		_, err := writer.Write(body)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return buffer.Bytes()
	}
	for _, item := range []struct {
		name, contentType, encoding string
		body                        []byte
		status                      int
	}{
		{"gzip-json-unknown-field", "application/json", "gzip", compress([]byte(`{"resourceMetrics":[],"futureField":true}`)), 200},
		{"invalid-gzip", "application/json", "gzip", []byte("bad"), 400},
		{"unsupported-encoding", "application/json", "br", []byte(`{}`), 415},
		{"unsupported-media", "text/plain", "", []byte(`{}`), 415},
		{"invalid-json", "application/json", "", []byte(`{`), 400},
		{"invalid-protobuf", "application/x-protobuf", "", []byte("invalid"), 400},
		{"gzip-bomb", "application/json", "gzip", compress([]byte(strings.Repeat(" ", (16<<20)+1))), 413},
	} {
		t.Run(item.name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/v1/metrics", bytes.NewReader(item.body))
			request.Header.Set("Content-Type", item.contentType)
			request.Header.Set("Content-Encoding", item.encoding)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, item.status, response.Code)
			if item.status != 200 {
				var status statuspb.Status
				if item.contentType == "application/json" {
					require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), &status))
				} else {
					require.NoError(t, proto.Unmarshal(response.Body.Bytes(), &status))
				}
				assert.NotEmpty(t, status.Message)
			}
		})
	}
}

func TestOTLPExpandedSampleLimitRejectsBeforeAnyWrite(t *testing.T) {
	db, handler := setup(t, "")
	points := make([]*metricspb.NumberDataPoint, 10001)
	for index := range points {
		points[index] = &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1}}
	}
	response, _ := exportOTLP(t, handler, []*metricspb.Metric{otlpGauge("oversized_otlp", points...)}, false)
	require.Equal(t, 413, response.Code)
	assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
	stored, err := db.Metrics(context.Background())
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func TestOTLPHistogramExpandedSampleBudgetUsesOptionalSum(t *testing.T) {
	for _, check := range []struct {
		name    string
		buckets int
		sum     bool
		prefix  bool
		status  int
	}{
		{"exact-without-sum", 9999, false, false, 200},
		{"exact-with-sum", 9998, true, false, 200},
		{"over-without-sum", 10000, false, false, 413},
		{"over-with-sum", 9999, true, false, 413},
		{"over-after-valid-prefix", 9999, false, true, 413},
	} {
		t.Run(check.name, func(t *testing.T) {
			db, handler := setup(t, "")
			backend := &interruptedIngestBackend{}
			db.Backend = backend
			point := &metricspb.HistogramDataPoint{BucketCounts: make([]uint64, check.buckets), ExplicitBounds: make([]float64, check.buckets-1)}
			for index := range point.ExplicitBounds {
				point.ExplicitBounds[index] = float64(index)
			}
			if check.sum {
				point.Sum = proto.Float64(0)
			}
			metrics := []*metricspb.Metric{{Name: "boundary_histogram", Data: &metricspb.Metric_Histogram{Histogram: &metricspb.Histogram{AggregationTemporality: metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricspb.HistogramDataPoint{point}}}}}
			if check.prefix {
				metrics = append([]*metricspb.Metric{otlpGauge("valid_prefix", &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1}})}, metrics...)
			}
			response, result := exportOTLP(t, handler, metrics, false)
			require.Equal(t, check.status, response.Code, response.Body.String())
			if check.status == 200 {
				assert.Nil(t, result.PartialSuccess)
				assert.Equal(t, "10000", response.Header().Get("X-Accepted-Samples"))
				assert.Equal(t, 10000, backend.accepted)
				assert.Equal(t, 1, backend.calls)
			} else {
				assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
				assert.Zero(t, backend.calls, "expanded-sample rejection must precede every storage call")
			}
		})
	}
}

func TestOTLPRejectsAttributeCollisionsAndStorageFailure(t *testing.T) {
	t.Run("normalization-collision", func(t *testing.T) {
		db, handler := setup(t, "")
		point := &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1}, Attributes: []*commonpb.KeyValue{
			{Key: "a.b", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "one"}}},
			{Key: "a_b", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "two"}}},
		}}
		response, result := exportOTLP(t, handler, []*metricspb.Metric{otlpGauge("collision", point)}, true)
		require.Equal(t, 200, response.Code)
		assert.Equal(t, int64(1), result.GetPartialSuccess().GetRejectedDataPoints())
		stored, err := db.Metrics(context.Background())
		require.NoError(t, err)
		assert.Empty(t, stored)
	})
	t.Run("storage-unavailable", func(t *testing.T) {
		db, handler := setup(t, "")
		backend := &interruptedIngestBackend{failAt: 1}
		db.Backend = backend
		response, _ := exportOTLP(t, handler, []*metricspb.Metric{otlpGauge("valid", &metricspb.NumberDataPoint{Value: &metricspb.NumberDataPoint_AsInt{AsInt: 1}})}, true)
		assert.Equal(t, 503, response.Code)
		assert.Equal(t, "0", response.Header().Get("X-Accepted-Samples"))
		assert.Zero(t, backend.accepted)
		var status statuspb.Status
		require.NoError(t, protojson.Unmarshal(response.Body.Bytes(), &status))
		assert.Contains(t, status.Message, "retry")
	})
}
