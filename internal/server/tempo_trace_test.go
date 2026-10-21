package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const tempoFixtureTraceID = "0123456789abcdef0123456789abcdef"

type tempoTraceQueryResponse struct {
	Results map[string]struct {
		Error  string `json:"error"`
		Frames []struct {
			Schema struct {
				Fields []struct{ Name string } `json:"fields"`
			} `json:"schema"`
			Data struct{ Values [][]any } `json:"data"`
		} `json:"frames"`
	} `json:"results"`
}

func queryTempoFixture(t *testing.T, body string, status int, traceID string) tempoTraceQueryResponse {
	t.Helper()
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/traces/"+traceID, r.URL.Path)
		assert.Empty(t, r.URL.RawQuery, "deep links must resolve outside the search window")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer tempo.Close()
	_, handler := setup(t, "")
	saved := call(handler, "POST", "/api/datasources", `{"uid":"tempo-detail","name":"Tempo detail","type":"tempo","url":"`+tempo.URL+`","access":"proxy"}`, "", "")
	require.Equal(t, 200, saved.Code, saved.Body.String())
	query := call(handler, "POST", "/api/ds/query", `{"from":"1700000000000","to":"1700000600000","queries":[{"refId":"A","datasource":{"uid":"tempo-detail"},"queryType":"traceId","traceId":"`+traceID+`"}]}`, "", "")
	require.Equal(t, 200, query.Code, query.Body.String())
	var response tempoTraceQueryResponse
	require.NoError(t, json.Unmarshal(query.Body.Bytes(), &response))
	return response
}

func TestTempoTraceLookupNormalizesHierarchyDurationAndStatus(t *testing.T) {
	fixture, err := os.ReadFile("testdata/tempo-trace.json")
	require.NoError(t, err)
	response := queryTempoFixture(t, string(fixture), 200, tempoFixtureTraceID)
	result := response.Results["A"]
	require.Empty(t, result.Error)
	require.Len(t, result.Frames, 1)
	frame := result.Frames[0]
	fields := map[string][]any{}
	for index, field := range frame.Schema.Fields {
		fields[field.Name] = frame.Data.Values[index]
	}
	assert.Equal(t, []any{"0000000000000001", "0000000000000002", "0000000000000003", "0000000000000004"}, fields["spanID"])
	assert.Equal(t, []any{"", "0000000000000001", "0000000000000002", "0000000000000001"}, fields["parentSpanID"])
	assert.Equal(t, []any{120.5, 25.25, float64(5), float64(10)}, fields["durationMs"])
	assert.Equal(t, []any{"ok", "error", "unset", "ok"}, fields["status"])
	assert.Equal(t, "database deadline exceeded", fields["statusMessage"][1])
	assert.Equal(t, "orders-db", fields["serviceName"][1])
	assert.Contains(t, fields["tags"][1], `"db.system.name":"mysql"`)
	assert.Equal(t, float64(1791633600010), fields["startTime"][1])
	for _, id := range fields["traceID"] {
		assert.Equal(t, tempoFixtureTraceID, id)
	}

	// Newer Tempo responses rename the outer collection to resourceSpans.
	response = queryTempoFixture(t, strings.Replace(string(fixture), `"batches"`, `"resourceSpans"`, 1), 200, tempoFixtureTraceID)
	assert.Empty(t, response.Results["A"].Error)
}

func TestTempoTraceLookupAcceptsUpstreamTempoFixture(t *testing.T) {
	fixture, err := os.ReadFile("testdata/tempo-upstream-trace.json")
	require.NoError(t, err)
	result := queryTempoFixture(t, string(fixture), 200, "e8b6dff9f8d44b28f1b22bc3bd2f6768").Results["A"]
	require.Empty(t, result.Error)
	require.Len(t, result.Frames, 1)
	values := result.Frames[0].Data.Values
	assert.Equal(t, []any{"9007fea484554ad7", "1fcbc1ba2ffd93eb", "2898383ecfa1a421"}, values[1])
	assert.Equal(t, []any{"", "9007fea484554ad7", "9007fea484554ad7"}, values[2])
	assert.InDelta(t, 15.317504, values[6][0], 1e-9)
	assert.Equal(t, []any{"error", "unset", "error"}, values[7])
}

func TestTempoTraceLookupFailureModes(t *testing.T) {
	fixture, err := os.ReadFile("testdata/tempo-trace.json")
	require.NoError(t, err)
	for _, item := range []struct {
		name, body, want string
		status           int
	}{
		{"empty", `{"batches":[]}`, "", 200},
		{"not-found", `not found`, "", 404},
		{"upstream-auth", `denied`, "HTTP 401", 401},
		{"upstream-failure", `unavailable`, "HTTP 503", 503},
		{"redirect", `redirect`, "HTTP 302", 302},
		{"malformed", `{`, "invalid Tempo trace response", 200},
		{"wrong-envelope", `{}`, "expected batches or resourceSpans", 200},
		{"invalid-id", strings.Replace(string(fixture), `"0000000000000001"`, `"not-an-id"`, 1), "invalid Tempo span ID", 200},
		{"invalid-time", strings.Replace(string(fixture), `1791633600120500000`, `1`, 1), "invalid Tempo span end time", 200},
		{"wrong-trace", strings.Replace(string(fixture), tempoFixtureTraceID, "1123456789abcdef0123456789abcdef", 1), "different or invalid trace ID", 200},
	} {
		t.Run(item.name, func(t *testing.T) {
			result := queryTempoFixture(t, item.body, item.status, tempoFixtureTraceID).Results["A"]
			if item.want == "" {
				require.Empty(t, result.Error)
				require.Len(t, result.Frames, 1)
				assert.Empty(t, result.Frames[0].Data.Values[0])
			} else {
				assert.Contains(t, result.Error, item.want)
			}
		})
	}
}

func TestTempoTraceLookupRejectsUnsafeIDsBeforeUpstream(t *testing.T) {
	var requests atomic.Int32
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1) }))
	defer tempo.Close()
	_, handler := setup(t, "")
	require.Equal(t, 200, call(handler, "POST", "/api/datasources", `{"uid":"tempo-invalid","name":"Tempo invalid","type":"tempo","url":"`+tempo.URL+`"}`, "", "").Code)
	for _, id := range []string{"", "../../ready", "abc?x=1", strings.Repeat("a", 33)} {
		payload, err := json.Marshal(map[string]any{"from": "0", "to": "1", "queries": []any{map[string]any{"refId": "A", "datasource": map[string]string{"uid": "tempo-invalid"}, "queryType": "traceId", "traceId": id}}})
		require.NoError(t, err)
		response := call(handler, "POST", "/api/ds/query", string(payload), "", "")
		assert.Contains(t, response.Body.String(), "1 to 32 hexadecimal")
	}
	assert.Zero(t, requests.Load())
}

func TestTempoTraceLookupCancelsUpstream(t *testing.T) {
	cancelled := make(chan struct{})
	tempo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		close(cancelled)
	}))
	defer tempo.Close()
	_, handler := setup(t, "")
	require.Equal(t, 200, call(handler, "POST", "/api/datasources", `{"uid":"tempo-cancel","name":"Tempo cancel","type":"tempo","url":"`+tempo.URL+`"}`, "", "").Code)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	request := httptest.NewRequest("POST", "/api/ds/query", strings.NewReader(`{"from":"0","to":"1","queries":[{"refId":"A","datasource":{"uid":"tempo-cancel"},"traceId":"`+tempoFixtureTraceID+`"}]}`)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("upstream was not cancelled")
	}
}

func TestTempoTraceLookupBoundsUniqueSpans(t *testing.T) {
	spans := make([]map[string]any, 10001)
	for index := range spans {
		spans[index] = map[string]any{"spanId": fmt.Sprintf("%016x", index+1), "name": "bounded fixture", "startTimeUnixNano": "1791633600000000000", "endTimeUnixNano": "1791633600000000001"}
	}
	encode := func() string {
		payload, err := json.Marshal(map[string]any{"resourceSpans": []any{map[string]any{"scopeSpans": []any{map[string]any{"spans": spans}}}}})
		require.NoError(t, err)
		return string(payload)
	}
	response := queryTempoFixture(t, encode(), 200, tempoFixtureTraceID)
	assert.Contains(t, response.Results["A"].Error, "exceeds 10000 spans")
	for index := range spans {
		spans[index]["spanId"] = "0000000000000001"
	}
	response = queryTempoFixture(t, encode(), 200, tempoFixtureTraceID)
	require.Empty(t, response.Results["A"].Error)
	require.Len(t, response.Results["A"].Frames, 1)
	assert.Len(t, response.Results["A"].Frames[0].Data.Values[0], 1, "repeated span batches are deduplicated")
}
