package server

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/neko233-com/metrics233-server/internal/model"
)

const maxTempoTraceSpans = 10000

// Tempo's v1 API uses batches/instrumentationLibrarySpans in older releases
// and resourceSpans/scopeSpans in newer OTLP JSON. IDs can be hex or protobuf
// JSON base64; normalize them before joining parent and child spans.
type tempoTraceDocument struct {
	Batches       []tempoResourceSpans `json:"batches"`
	ResourceSpans []tempoResourceSpans `json:"resourceSpans"`
}

type tempoResourceSpans struct {
	Resource struct {
		Attributes []tempoAttribute `json:"attributes"`
	} `json:"resource"`
	ScopeSpans                  []tempoScopeSpans `json:"scopeSpans"`
	InstrumentationLibrarySpans []tempoScopeSpans `json:"instrumentationLibrarySpans"`
}

type tempoScopeSpans struct {
	Spans []tempoSpan `json:"spans"`
}

type tempoAttribute struct {
	Key   string         `json:"key"`
	Value map[string]any `json:"value"`
}

type tempoSpan struct {
	TraceID           string           `json:"traceId"`
	SpanID            string           `json:"spanId"`
	ParentSpanID      string           `json:"parentSpanId"`
	Name              string           `json:"name"`
	StartTimeUnixNano json.RawMessage  `json:"startTimeUnixNano"`
	EndTimeUnixNano   json.RawMessage  `json:"endTimeUnixNano"`
	Kind              json.RawMessage  `json:"kind"`
	Attributes        []tempoAttribute `json:"attributes"`
	Status            struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	} `json:"status"`
}

func (s *Server) queryTempoTrace(ctx context.Context, ds model.DataSource, query backend.DataQuery, traceID string) (data.Frames, error) {
	if traceID == "" || len(traceID) > 32 || strings.Trim(traceID, "0123456789abcdefABCDEF") != "" {
		return nil, errors.New("Tempo trace ID must contain 1 to 32 hexadecimal characters")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://datasource/api/traces/"+traceID, nil)
	if err != nil {
		return nil, err
	}
	// Do not restrict by the dashboard range: a persisted deep link must still
	// resolve when its trace falls outside the currently selected search window.
	output := &boundedResponse{header: http.Header{}}
	s.proxyDataSource(output, request, ds)
	if output.status == http.StatusNotFound {
		return data.Frames{newTempoSpanFrame(query.RefID, nil)}, nil
	}
	if output.status != http.StatusOK {
		return nil, fmt.Errorf("Tempo trace lookup failed with HTTP %d", output.status)
	}
	var document tempoTraceDocument
	if err := json.Unmarshal(output.body.Bytes(), &document); err != nil {
		return nil, fmt.Errorf("invalid Tempo trace response: %w", err)
	}
	if document.Batches == nil && document.ResourceSpans == nil {
		return nil, errors.New("invalid Tempo trace response: expected batches or resourceSpans")
	}
	rows := []tempoSpanRow{}
	seen := map[string]bool{}
	wantedID := strings.Repeat("0", 32-len(traceID)) + strings.ToLower(traceID)
	for _, resource := range append(document.Batches, document.ResourceSpans...) {
		resourceTags := tempoAttributes(resource.Resource.Attributes)
		service, _ := resourceTags["service.name"].(string)
		for _, scope := range append(resource.ScopeSpans, resource.InstrumentationLibrarySpans...) {
			for _, span := range scope.Spans {
				id, err := tempoIdentifier(span.SpanID, 8)
				if err != nil {
					return nil, fmt.Errorf("invalid Tempo span ID: %w", err)
				}
				parent := ""
				if span.ParentSpanID != "" {
					parent, err = tempoIdentifier(span.ParentSpanID, 8)
					if err != nil {
						return nil, fmt.Errorf("invalid Tempo parent span ID: %w", err)
					}
				}
				if span.TraceID != "" {
					gotID, err := tempoIdentifier(span.TraceID, 16)
					if err != nil || gotID != wantedID {
						return nil, errors.New("Tempo response contains a different or invalid trace ID")
					}
				}
				if seen[id] {
					continue
				}
				if len(rows) >= maxTempoTraceSpans {
					return nil, fmt.Errorf("Tempo trace exceeds %d spans", maxTempoTraceSpans)
				}
				start, err := tempoNanoseconds(span.StartTimeUnixNano)
				if err != nil {
					return nil, errors.New("invalid Tempo span start time")
				}
				end, err := tempoNanoseconds(span.EndTimeUnixNano)
				if err != nil || end < start {
					return nil, errors.New("invalid Tempo span end time")
				}
				tags := tempoAttributes(resource.Resource.Attributes)
				for key, value := range tempoAttributes(span.Attributes) {
					tags[key] = value
				}
				status := "unset"
				code := strings.Trim(string(span.Status.Code), "\"")
				if code == "2" || code == "STATUS_CODE_ERROR" || tags["error"] == true {
					status = "error"
				} else if code == "1" || code == "STATUS_CODE_OK" {
					status = "ok"
				}
				tagJSON, _ := json.Marshal(tags)
				rows = append(rows, tempoSpanRow{traceID: wantedID, id: id, parent: parent, service: service,
					name: span.Name, start: time.Unix(0, start), duration: float64(end-start) / 1e6,
					status: status, message: span.Status.Message, kind: strings.Trim(string(span.Kind), "\""), tags: string(tagJSON)})
				seen[id] = true
			}
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].start.Before(rows[j].start) })
	return data.Frames{newTempoSpanFrame(query.RefID, rows)}, nil
}

func tempoIdentifier(value string, size int) (string, error) {
	if len(value) == size*2 {
		if decoded, err := hex.DecodeString(value); err == nil {
			return hex.EncodeToString(decoded), nil
		}
	}
	if decoded, err := base64.StdEncoding.DecodeString(value); err == nil && len(decoded) == size {
		return hex.EncodeToString(decoded), nil
	}
	return "", errors.New("expected hexadecimal or base64 ID")
}

func tempoNanoseconds(raw json.RawMessage) (int64, error) {
	value, err := strconv.ParseInt(strings.Trim(string(raw), "\""), 10, 64)
	if err != nil || value <= 0 {
		return 0, errors.New("invalid nanosecond timestamp")
	}
	return value, nil
}

func tempoAttributes(attributes []tempoAttribute) map[string]any {
	result := make(map[string]any, len(attributes))
	for _, attribute := range attributes {
		for _, key := range []string{"stringValue", "boolValue", "intValue", "doubleValue", "arrayValue", "kvlistValue", "bytesValue"} {
			if value, ok := attribute.Value[key]; ok {
				result[attribute.Key] = value
				break
			}
		}
	}
	return result
}

type tempoSpanRow struct {
	traceID, id, parent, service, name, status, message, kind, tags string
	start                                                           time.Time
	duration                                                        float64
}

func newTempoSpanFrame(refID string, rows []tempoSpanRow) *data.Frame {
	traceIDs, ids, parents, services, names := []string{}, []string{}, []string{}, []string{}, []string{}
	starts, durations := []time.Time{}, []float64{}
	statuses, messages, kinds, tags := []string{}, []string{}, []string{}, []string{}
	for _, row := range rows {
		traceIDs, ids, parents = append(traceIDs, row.traceID), append(ids, row.id), append(parents, row.parent)
		services, names = append(services, row.service), append(names, row.name)
		starts, durations = append(starts, row.start), append(durations, row.duration)
		statuses, messages = append(statuses, row.status), append(messages, row.message)
		kinds, tags = append(kinds, row.kind), append(tags, row.tags)
	}
	frame := data.NewFrame("Trace spans", data.NewField("traceID", nil, traceIDs), data.NewField("spanID", nil, ids),
		data.NewField("parentSpanID", nil, parents), data.NewField("serviceName", nil, services), data.NewField("operationName", nil, names),
		data.NewField("startTime", nil, starts), data.NewField("durationMs", nil, durations), data.NewField("status", nil, statuses),
		data.NewField("statusMessage", nil, messages), data.NewField("kind", nil, kinds), data.NewField("tags", nil, tags))
	frame.RefID = refID
	frame.Meta = &data.FrameMeta{PreferredVisualization: data.VisType("traces")}
	return frame
}
