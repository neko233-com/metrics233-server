package server

import (
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
)

const otlpMaxSamples = 10000

type otlpConversion struct {
	samples  []model.Sample
	rejected int64
	reason   string
	now      int64
}

func (result *otlpConversion) reject(count int, reason string) {
	result.rejected += int64(count)
	if count > 0 && result.reason == "" {
		result.reason = reason
	}
}

func otlpSamples(resources []*metricspb.ResourceMetrics) (*otlpConversion, error) {
	result := &otlpConversion{now: time.Now().UnixMilli()}
	for _, resource := range resources {
		base := otlpAttributes(resource.GetResource())
		baseError := validateOTLPAttributes(resource.GetResource().GetAttributes())
		for _, scope := range resource.GetScopeMetrics() {
			labels := otlpLabels(base, nil)
			if name := scope.GetScope().GetName(); name != "" {
				labels["otel_scope_name"] = name
			}
			if version := scope.GetScope().GetVersion(); version != "" {
				labels["otel_scope_version"] = version
			}
			for _, metric := range scope.GetMetrics() {
				if err := result.metric(metric, labels, baseError); err != nil {
					return nil, err
				}
			}
		}
	}
	return result, nil
}

type otlpPoint interface {
	GetAttributes() []*commonpb.KeyValue
	GetTimeUnixNano() uint64
	GetFlags() uint32
}

// Validate each original data point before appending any of its derived series.
// This keeps a rejected histogram/summary indivisible and reports OTLP point
// counts rather than counting the expanded classic bucket samples as points.
func (result *otlpConversion) point(name string, base map[string]string, point otlpPoint, reason string, build func(map[string]string) []model.Sample) error {
	if name == "" && reason == "" {
		reason = "metric name is empty"
	}
	if reason == "" {
		reason = validateOTLPAttributes(point.GetAttributes())
	}
	if point.GetFlags()&uint32(metricspb.DataPointFlags_DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK) != 0 {
		reason = "no-recorded-value markers require staleness storage and are not supported"
	}
	if reason != "" {
		result.reject(1, reason)
		return nil
	}
	samples := build(otlpLabels(base, point.GetAttributes()))
	timestamp := result.now
	if point.GetTimeUnixNano() > 0 {
		timestamp = int64(point.GetTimeUnixNano() / 1_000_000)
	}
	for index := range samples {
		samples[index].Timestamp = timestamp
		if err := samples[index].Validate(); err != nil {
			result.reject(1, "data point exceeds metric name, label, finite-value or timestamp constraints")
			return nil
		}
	}
	if len(result.samples)+len(samples) > otlpMaxSamples {
		return errors.New("OTLP request exceeds 10000 expanded samples")
	}
	result.samples = append(result.samples, samples...)
	return nil
}

func (result *otlpConversion) metric(metric *metricspb.Metric, base map[string]string, baseError string) error {
	name := sanitizeMetricName(metric.GetName())
	if gauge := metric.GetGauge(); gauge != nil {
		for _, point := range gauge.GetDataPoints() {
			if err := result.number(name, base, point, baseError, false); err != nil {
				return err
			}
		}
	}
	if sum := metric.GetSum(); sum != nil {
		if sum.GetAggregationTemporality() != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
			result.reject(len(sum.GetDataPoints()), "sum requires cumulative temporality; delta accumulation is not supported")
			return nil
		}
		for _, point := range sum.GetDataPoints() {
			if err := result.number(name, base, point, baseError, sum.GetIsMonotonic()); err != nil {
				return err
			}
		}
	}
	if histogram := metric.GetHistogram(); histogram != nil {
		if histogram.GetAggregationTemporality() != metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE {
			result.reject(len(histogram.GetDataPoints()), "histogram requires cumulative temporality; delta accumulation is not supported")
			return nil
		}
		for _, point := range histogram.GetDataPoints() {
			reason := baseError
			if reason == "" {
				reason = validateOTLPHistogram(point)
			}
			expanded := len(point.GetBucketCounts()) + 1 // Buckets and the required count.
			if point.Sum != nil {
				expanded++
			}
			if reason == "" && expanded > otlpMaxSamples {
				return errors.New("OTLP histogram exceeds 10000 expanded samples")
			}
			if err := result.point(name, base, point, reason, func(labels map[string]string) []model.Sample {
				samples := make([]model.Sample, 0, expanded)
				var cumulative uint64
				for index, count := range point.GetBucketCounts() {
					cumulative += count
					bucket := otlpLabels(labels, nil)
					bucket["le"] = "+Inf"
					if index < len(point.GetExplicitBounds()) {
						bucket["le"] = strconv.FormatFloat(point.GetExplicitBounds()[index], 'g', -1, 64)
					}
					samples = append(samples, model.Sample{Name: name + "_bucket", Labels: bucket, Value: float64(cumulative)})
				}
				samples = append(samples, model.Sample{Name: name + "_count", Labels: labels, Value: float64(point.GetCount())})
				if point.Sum != nil {
					samples = append(samples, model.Sample{Name: name + "_sum", Labels: labels, Value: point.GetSum()})
				}
				return samples
			}); err != nil {
				return err
			}
		}
	}
	if histogram := metric.GetExponentialHistogram(); histogram != nil {
		result.reject(len(histogram.GetDataPoints()), "exponential histograms are not supported; export explicit cumulative histograms")
	}
	if summary := metric.GetSummary(); summary != nil {
		for _, point := range summary.GetDataPoints() {
			reason := baseError
			previous := -1.0
			for _, quantile := range point.GetQuantileValues() {
				if quantile == nil || !finiteOTLP(quantile.GetQuantile()) || quantile.GetQuantile() < 0 || quantile.GetQuantile() <= previous || quantile.GetQuantile() > 1 || quantile.GetValue() < 0 {
					reason = "summary quantiles must be finite, strictly increasing and within [0,1] with non-negative values"
				}
				previous = quantile.GetQuantile()
			}
			if point.GetCount() == 0 && point.GetSum() != 0 {
				reason = "summary sum must be zero for an empty population"
			}
			if reason == "" && len(point.GetQuantileValues())+2 > otlpMaxSamples {
				return errors.New("OTLP summary exceeds 10000 expanded samples")
			}
			if err := result.point(name, base, point, reason, func(labels map[string]string) []model.Sample {
				samples := []model.Sample{{Name: name + "_count", Labels: labels, Value: float64(point.GetCount())}, {Name: name + "_sum", Labels: labels, Value: point.GetSum()}}
				for _, quantile := range point.GetQuantileValues() {
					quantileLabels := otlpLabels(labels, nil)
					quantileLabels["quantile"] = strconv.FormatFloat(quantile.GetQuantile(), 'g', -1, 64)
					samples = append(samples, model.Sample{Name: name, Labels: quantileLabels, Value: quantile.GetValue()})
				}
				return samples
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (result *otlpConversion) number(name string, base map[string]string, point *metricspb.NumberDataPoint, reason string, monotonic bool) error {
	value := point.GetAsDouble()
	switch typed := point.GetValue().(type) {
	case *metricspb.NumberDataPoint_AsInt:
		value = float64(typed.AsInt)
	case *metricspb.NumberDataPoint_AsDouble:
	default:
		reason = "number data point is missing its value"
	}
	if monotonic && value < 0 {
		reason = "monotonic cumulative sum must not be negative"
	}
	return result.point(name, base, point, reason, func(labels map[string]string) []model.Sample {
		return []model.Sample{{Name: name, Labels: labels, Value: value}}
	})
}

func validateOTLPHistogram(point *metricspb.HistogramDataPoint) string {
	if point == nil {
		return "histogram data point is missing"
	}
	counts, bounds := point.GetBucketCounts(), point.GetExplicitBounds()
	if (len(counts) == 0 && len(bounds) != 0) || (len(counts) > 0 && len(counts) != len(bounds)+1) {
		return "histogram bucket counts must have exactly one more element than explicit bounds"
	}
	for index, bound := range bounds {
		if !finiteOTLP(bound) || (index > 0 && bound <= bounds[index-1]) {
			return "histogram bounds must be finite and strictly increasing"
		}
	}
	var total uint64
	for _, count := range counts {
		if count > point.GetCount()-total {
			return "histogram bucket counts do not match count"
		}
		total += count
	}
	if len(counts) > 0 && total != point.GetCount() {
		return "histogram bucket counts do not match count"
	}
	if point.Sum != nil && (!finiteOTLP(point.GetSum()) || (point.GetCount() == 0 && point.GetSum() != 0)) {
		return "histogram sum must be finite and zero for an empty population"
	}
	return ""
}

func finiteOTLP(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validateOTLPAttributes(attributes []*commonpb.KeyValue) string {
	keys := make(map[string]bool, len(attributes))
	for _, attribute := range attributes {
		key := sanitizeLabelKey(attribute.GetKey())
		if key == "" || keys[key] {
			return "attribute names must be non-empty and unique after label normalization"
		}
		keys[key] = true
		if _, supported := otlpAttributeValue(attribute.GetValue()); !supported {
			return "attribute has an empty or unsupported value"
		}
	}
	return ""
}
