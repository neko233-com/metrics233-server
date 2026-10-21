package server

import (
	"compress/gzip"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	collectormetrics "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const otlpBodyLimit = 16 << 20

// OTLP/HTTP metrics receiver. Prometheus scraping and native SQL/Redis
// collectors remain separate; all three write into the same metric store.
func (s *Server) otlpMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Accepted-Samples", "0")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		otlpFail(w, r, http.StatusMethodNotAllowed, errors.New("OTLP metrics endpoint accepts POST only"))
		return
	}
	media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if r.Header.Get("Content-Type") != "" && (mediaErr != nil || (media != "application/json" && media != "application/x-protobuf")) {
		otlpFail(w, r, http.StatusUnsupportedMediaType, errors.New("OTLP metrics requires application/json or application/x-protobuf"))
		return
	}
	body, err := readOTLPBody(r)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, errOTLPBodyLimit) {
			status = http.StatusRequestEntityTooLarge
		} else if errors.Is(err, errOTLPEncoding) {
			status = http.StatusUnsupportedMediaType
		}
		otlpFail(w, r, status, err)
		return
	}
	jsonRequest := media == "application/json"
	var request collectormetrics.ExportMetricsServiceRequest
	if jsonRequest {
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, &request); err != nil {
			otlpFail(w, r, http.StatusBadRequest, errors.New("invalid OTLP JSON"))
			return
		}
	} else if err := proto.Unmarshal(body, &request); err != nil {
		otlpFail(w, r, http.StatusBadRequest, errors.New("invalid OTLP protobuf"))
		return
	}
	converted, err := otlpSamples(request.ResourceMetrics)
	if err != nil {
		otlpFail(w, r, http.StatusRequestEntityTooLarge, err)
		return
	}
	samples := converted.samples
	if len(samples) > 0 {
		if err := s.Store.Ingest(r.Context(), samples); err != nil {
			otlpFail(w, r, http.StatusServiceUnavailable, errors.New("metric storage unavailable; retry the request"))
			return
		}
		w.Header().Set("X-Accepted-Samples", strconv.Itoa(len(samples)))
	}
	response := &collectormetrics.ExportMetricsServiceResponse{}
	if converted.rejected > 0 {
		response.PartialSuccess = &collectormetrics.ExportMetricsPartialSuccess{RejectedDataPoints: converted.rejected, ErrorMessage: converted.reason}
	}
	if jsonRequest {
		w.Header().Set("Content-Type", "application/json")
		data, _ := protojson.Marshal(response)
		_, _ = w.Write(data)
		return
	}
	data, _ := proto.Marshal(response)
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, _ = w.Write(data)
}

var errOTLPBodyLimit = errors.New("OTLP request exceeds 16 MiB")
var errOTLPEncoding = errors.New("unsupported OTLP Content-Encoding; use gzip or identity")

func otlpFail(w http.ResponseWriter, r *http.Request, code int, err error) {
	response := &statuspb.Status{Message: err.Error()}
	media, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	var body []byte
	if media == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		body, _ = protojson.Marshal(response)
	} else {
		w.Header().Set("Content-Type", "application/x-protobuf")
		body, _ = proto.Marshal(response)
	}
	w.WriteHeader(code)
	_, _ = w.Write(body)
}

func readOTLPBody(r *http.Request) ([]byte, error) {
	raw := &io.LimitedReader{R: r.Body, N: otlpBodyLimit + 1}
	var reader io.Reader = raw
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gzipReader, err := gzip.NewReader(raw)
		if err != nil {
			return nil, errors.New("invalid gzip OTLP body")
		}
		defer gzipReader.Close()
		reader = gzipReader
	default:
		return nil, errOTLPEncoding
	}
	data, err := io.ReadAll(io.LimitReader(reader, otlpBodyLimit+1))
	if len(data) > otlpBodyLimit || raw.N == 0 {
		return nil, errOTLPBodyLimit
	}
	if err != nil {
		return nil, errors.New("invalid or truncated OTLP body")
	}
	return data, nil
}

func otlpLabels(base map[string]string, attributes []*commonpb.KeyValue) map[string]string {
	labels := make(map[string]string, len(base)+len(attributes))
	for key, value := range base {
		labels[key] = value
	}
	for _, attribute := range attributes {
		key := sanitizeLabelKey(attribute.GetKey())
		if key == "" {
			continue
		}
		if value, ok := otlpAttributeValue(attribute.GetValue()); ok {
			labels[key] = value
		}
	}
	return labels
}

func otlpAttributes(resource *resourcepb.Resource) map[string]string {
	if resource == nil {
		return map[string]string{}
	}
	return otlpLabels(nil, resource.GetAttributes())
}

func otlpAttributeValue(value *commonpb.AnyValue) (string, bool) {
	if value == nil {
		return "", false
	}
	switch typed := value.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue, true
	case *commonpb.AnyValue_BoolValue:
		return strconv.FormatBool(typed.BoolValue), true
	case *commonpb.AnyValue_IntValue:
		return strconv.FormatInt(typed.IntValue, 10), true
	case *commonpb.AnyValue_DoubleValue:
		return strconv.FormatFloat(typed.DoubleValue, 'f', -1, 64), true
	case *commonpb.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(typed.BytesValue), true
	case *commonpb.AnyValue_ArrayValue:
		parts := []string{}
		for _, item := range typed.ArrayValue.GetValues() {
			if text, ok := otlpAttributeValue(item); ok {
				parts = append(parts, text)
			} else {
				return "", false
			}
		}
		return strings.Join(parts, ","), true
	default:
		return "", false
	}
}

func sanitizeMetricName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for index, character := range value {
		valid := character == '_' || character == ':' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			index > 0 && character >= '0' && character <= '9'
		if valid {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	name := builder.String()
	if name == "" || name[0] >= '0' && name[0] <= '9' {
		name = "_" + name
	}
	return name
}

func sanitizeLabelKey(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var builder strings.Builder
	for index, character := range value {
		valid := character == '_' ||
			character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			index > 0 && character >= '0' && character <= '9'
		if valid {
			builder.WriteRune(character)
		} else {
			builder.WriteByte('_')
		}
	}
	key := builder.String()
	if key == "" || key[0] >= '0' && key[0] <= '9' {
		key = "_" + key
	}
	return key
}
