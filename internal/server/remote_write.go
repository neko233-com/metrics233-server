package server

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	promvalue "github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"

	"github.com/neko233-com/metrics233-server/internal/model"
)

const (
	remoteWriteBodyLimit    = 32 << 20
	remoteWriteDecodedLimit = 64 << 20
	remoteWriteMaxSamples   = 100000
)

// remoteWrite accepts Prometheus remote_write v1 requests. Pull scraping and
// push receivers share the same metric store and ingest role policy.
func (s *Server) remoteWrite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Accepted-Samples", "0")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fail(w, http.StatusMethodNotAllowed, errors.New("remote write endpoint accepts POST only"))
		return
	}
	if contentType := r.Header.Get("Content-Type"); contentType != "" {
		media, params, err := mime.ParseMediaType(contentType)
		if err != nil || media != "application/x-protobuf" || (params["proto"] != "" && params["proto"] != "prometheus.WriteRequest") {
			fail(w, http.StatusUnsupportedMediaType, errors.New("remote write accepts prometheus.WriteRequest protobuf only; v2 is not supported"))
			return
		}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, remoteWriteBodyLimit))
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			fail(w, http.StatusRequestEntityTooLarge, errors.New("remote write request exceeds 32 MiB"))
		} else {
			fail(w, http.StatusBadRequest, errors.New("could not read remote write body"))
		}
		return
	}
	raw := body
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "snappy":
		decodedLength, decodeErr := snappy.DecodedLen(body)
		if decodeErr != nil {
			fail(w, http.StatusBadRequest, errors.New("invalid snappy remote write body"))
			return
		}
		if decodedLength > remoteWriteDecodedLimit {
			fail(w, http.StatusRequestEntityTooLarge, errors.New("remote write body exceeds 64 MiB after decoding"))
			return
		}
		decoded, decodeErr := snappy.Decode(nil, body)
		if decodeErr != nil {
			fail(w, http.StatusBadRequest, errors.New("invalid snappy remote write body"))
			return
		}
		raw = decoded
	default:
		fail(w, http.StatusUnsupportedMediaType, fmt.Errorf("unsupported remote write Content-Encoding %q", r.Header.Get("Content-Encoding")))
		return
	}
	if len(raw) > remoteWriteDecodedLimit {
		fail(w, http.StatusRequestEntityTooLarge, errors.New("remote write body exceeds 64 MiB after decoding"))
		return
	}
	var request prompb.WriteRequest
	if err := proto.Unmarshal(raw, &request); err != nil {
		fail(w, http.StatusBadRequest, fmt.Errorf("invalid remote write protobuf: %w", err))
		return
	}
	samples, err := remoteWriteSamples(request.Timeseries)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errRemoteWriteSampleLimit) {
			status = http.StatusRequestEntityTooLarge
		}
		fail(w, status, err)
		return
	}
	for offset := 0; offset < len(samples); offset += 10000 {
		end := min(offset+10000, len(samples))
		if err := s.ingestReported(r.Context(), samples[offset:end]); err != nil {
			fail(w, http.StatusServiceUnavailable, errors.New("metric storage unavailable; retry the request"))
			return
		}
		w.Header().Set("X-Accepted-Samples", strconv.Itoa(end))
	}
	w.Header().Set("X-Accepted-Samples", strconv.Itoa(len(samples)))
	w.WriteHeader(http.StatusNoContent)
}

var errRemoteWriteSampleLimit = errors.New("remote write request exceeds 100000 samples")

func remoteWriteSamples(series []prompb.TimeSeries) ([]model.Sample, error) {
	samples := make([]model.Sample, 0, min(len(series), remoteWriteMaxSamples))
	points := 0
	for _, item := range series {
		points += len(item.Samples)
		if points > remoteWriteMaxSamples {
			return nil, errRemoteWriteSampleLimit
		}
		if len(item.Histograms) > 0 {
			return nil, errors.New("remote write native histograms require histogram storage; send classic bucket series")
		}
		name := ""
		labels := make(map[string]string, len(item.Labels))
		seen := make(map[string]bool, len(item.Labels))
		for _, label := range item.Labels {
			if seen[label.Name] {
				return nil, errors.New("remote write series contains duplicate labels")
			}
			seen[label.Name] = true
			if label.Name == "__name__" {
				name = label.Value
				continue
			}
			labels[label.Name] = label.Value
		}
		if name == "" {
			if len(item.Samples) > 0 {
				return nil, errors.New("remote write samples require a non-empty metric name (__name__)")
			}
			continue
		}
		for _, sample := range item.Samples {
			if promvalue.IsStaleNaN(sample.Value) {
				return nil, errors.New("remote write stale markers require staleness storage; this receiver accepts finite samples only")
			}
			point := model.Sample{
				Name:      name,
				Labels:    labels,
				Value:     sample.Value,
				Timestamp: sample.Timestamp,
			}
			if err := point.Validate(); err != nil {
				return nil, fmt.Errorf("invalid remote write sample: %w", err)
			}
			samples = append(samples, point)
		}
	}
	return samples, nil
}
