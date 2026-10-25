package server_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/neko233-com/metrics233-server/internal/store"
	"github.com/stretchr/testify/require"
)

func TestBusinessNetworkObservedRecommendationAndMissingManualSelection(t *testing.T) {
	db, handler := setup(t, "")
	ctx := context.Background()
	host, err := db.SaveManagedHost(ctx, model.ManagedHost{Name: "Network fixture", Address: "127.0.0.1", Port: 22, User: "fixture", Enabled: true, Target: model.Target{Name: "network", Kind: "mysql", URL: "mysql://127.0.0.1:3306", IntervalSeconds: 15, Enabled: true}}, "fixture")
	require.NoError(t, err)
	identity := model.BusinessIdentity{Service: "network-fixture", Role: "logic", ServerID: "7", Instance: "network-7", Environment: "test", Implementation: "go"}
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: identity.Labels(), Value: 0}}))
	id := store.BusinessInstanceID(identity)
	_, err = db.BindBusinessHost(ctx, id, model.BusinessHostBinding{Kind: "managed_host", ID: host.ID})
	require.NoError(t, err)
	now := time.Now().Add(-time.Second).UnixMilli()
	observations := []model.Sample{}
	for device, rate := range map[string]float64{"anpi0": 0, "bridge0": 9000, "en0": 10, "en1": 20} {
		observations = append(observations, model.Sample{Name: "host_network_receive_bytes_per_second", Labels: map[string]string{"device": device}, Value: rate, Timestamp: now})
	}
	require.NoError(t, db.RecordHostHeartbeat(ctx, host.ID, model.HostHeartbeat{Samples: observations}, len(observations)))
	type response struct {
		Device      string          `json:"device"`
		Recommended string          `json:"recommended_device"`
		Traffic     bool            `json:"recommendation_has_traffic"`
		Stale       bool            `json:"host_stale"`
		Host        model.Dashboard `json:"host"`
	}
	read := func(query string) response {
		w := call(handler, "GET", "/api/v1/business/instances/"+id+"/network"+query, "", "", "")
		require.Equal(t, 200, w.Code, w.Body.String())
		var result response
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
		return result
	}
	auto := read("")
	require.Equal(t, "en1", auto.Device)
	require.Equal(t, "en1", auto.Recommended)
	require.True(t, auto.Traffic)
	require.Len(t, auto.Host.Panels, 3)
	manual := read("?device=anpi0")
	require.Equal(t, "anpi0", manual.Device)
	missing := read("?device=removed0")
	require.Equal(t, "removed0", missing.Device)
	require.Equal(t, "en1", missing.Recommended)
	require.False(t, missing.Stale)
	require.Len(t, missing.Host.Panels, 1)
	require.Equal(t, "timeseries", missing.Host.Panels[0].Visualization)
	require.Contains(t, missing.Host.Panels[0].Expr, `device="removed0"`)
	for _, bad := range []string{"a\n", "\x00", strings.Repeat("x", 129)} {
		require.Equal(t, 400, call(handler, "GET", "/api/v1/business/instances/"+id+"/network?device="+url.QueryEscape(bad), "", "", "").Code)
	}
	require.Equal(t, 400, call(handler, "GET", "/api/v1/business/instances/"+id+"/network?device=en0&device=en1", "", "", "").Code)
	for n := range observations {
		observations[n].Timestamp = time.Now().Add(-3 * time.Minute).UnixMilli()
	}
	require.NoError(t, db.RecordHostHeartbeat(ctx, host.ID, model.HostHeartbeat{Samples: observations}, len(observations)))
	stale := read("")
	require.True(t, stale.Stale)
	require.False(t, stale.Traffic)
	require.Equal(t, "en0", stale.Device)
	require.Len(t, stale.Host.Panels, 1)
}

func TestBusinessNetworkCollectorRecommendationExcludesOldAndForeignInterfaces(t *testing.T) {
	db, handler := setup(t, "")
	ctx := context.Background()
	identity := model.BusinessIdentity{Service: "collector-fixture", Role: "logic", ServerID: "8", Instance: "collector-8", Environment: "test", Implementation: "go"}
	require.NoError(t, db.Ingest(ctx, []model.Sample{{Name: "game_players_online", Labels: identity.Labels(), Value: 0}}))
	id := store.BusinessInstanceID(identity)
	target, err := db.SaveTarget(ctx, model.Target{Name: "nic-fixture", URL: "http://127.0.0.1:19090/metrics", IntervalSeconds: 15, Enabled: true, Labels: map[string]string{"instance": "nic-fixture"}})
	require.NoError(t, err)
	_, err = db.BindBusinessHost(ctx, id, model.BusinessHostBinding{Kind: "collector", ID: strconv.FormatInt(target.ID, 10)})
	require.NoError(t, err)
	now := time.Now().Add(-time.Second)
	_, err = db.DB.ExecContext(ctx, `UPDATE targets SET last_scrape=? WHERE id=?`, now.UnixMilli(), target.ID)
	require.NoError(t, err)
	observations := []model.Sample{}
	for _, device := range []string{"eth0", "eth1", "eth2", "foreign"} {
		for n := 0; n < 4; n++ {
			at, job := now.Add(time.Duration(n-3)*30*time.Second), "nic-fixture"
			if device == "eth2" {
				at = at.Add(-3 * time.Minute)
			}
			if device == "foreign" {
				job = "another-host"
			}
			value := 0.0
			if device != "eth0" {
				value = float64(n * 300)
			}
			observations = append(observations, model.Sample{Name: "node_network_receive_bytes_total", Labels: map[string]string{"job": job, "instance": "nic-fixture", "device": device}, Value: value, Timestamp: at.UnixMilli()})
		}
	}
	require.NoError(t, db.Ingest(ctx, observations))
	w := call(handler, "GET", "/api/v1/business/instances/"+id+"/network", "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var result struct {
		Device  string   `json:"device"`
		Devices []string `json:"devices"`
		Traffic bool     `json:"recommendation_has_traffic"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, []string{"eth0", "eth1"}, result.Devices)
	require.Equal(t, "eth1", result.Device)
	require.True(t, result.Traffic)
}

func TestBusinessDirectoryNetworkAliasesScopeAndCollectorQueries(t *testing.T) {
	db, h := setup(t, "")
	ctx := context.Background()
	now := time.Now().Add(-time.Second)
	i := model.BusinessIdentity{Service: "fixture", Role: "logic", ServerID: "7", Instance: "logic-7", Environment: "test", Implementation: "go"}
	samples := []model.Sample{}
	for _, direction := range []string{"received", "sent"} {
		for _, metric := range []string{"service_payload_" + direction + "_bytes_total", "game_bytes_" + direction + "_total"} {
			for n := 0; n < 5; n++ {
				value := float64(n * 300)
				if metric[0] == 'g' {
					value *= 10
				}
				samples = append(samples, model.Sample{Name: metric, Labels: i.Labels(), Value: value, Timestamp: now.Add(time.Duration(n-4) * 30 * time.Second).UnixMilli()})
			}
		}
	}
	require.NoError(t, db.Ingest(ctx, samples))
	id := store.BusinessInstanceID(i)
	path := "/api/v1/business/instances/" + id
	w := call(h, "GET", "/api/v1/business/directory?role=game&environment=test&sort=server", "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var directory struct {
		Summaries map[string]struct {
			Receive, Transmit *float64 `json:"-"`
			In                *float64 `json:"receive_per_second"`
			Out               *float64 `json:"transmit_per_second"`
		} `json:"summaries"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &directory))
	require.NotNil(t, directory.Summaries[id].In)
	require.InDelta(t, 10, *directory.Summaries[id].In, 0.01, "unified and legacy aliases must not be added")
	note := call(h, "PUT", path+"/note", `{"version":0,"description":"夜间重点观察\n<script>plain text</script>"}`, "", "")
	require.Equal(t, 200, note.Code, note.Body.String())
	require.Equal(t, 409, call(h, "PUT", path+"/note", `{"version":0,"description":"overwrite"}`, "", "").Code)
	w = call(h, "GET", path+"/network?language=zh", "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var view struct {
		Business model.Dashboard `json:"business"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &view))
	require.Len(t, view.Business.Panels, 5)
	for _, p := range view.Business.Panels {
		q := call(h, "GET", "/prometheus/api/v1/query?query="+url.QueryEscape(p.Expr), "", "", "")
		require.Equal(t, 200, q.Code, q.Body.String())
	}
	// A real node-exporter identity is explicitly bound, never inferred from IP.
	target, err := db.SaveTarget(ctx, model.Target{Name: "host-fixture", URL: "http://127.0.0.1:19090/metrics", IntervalSeconds: 15, Enabled: true, Labels: map[string]string{"instance": "host-fixture"}})
	require.NoError(t, err)
	_, err = db.BindBusinessHost(ctx, id, model.BusinessHostBinding{Kind: "collector", ID: strconv.FormatInt(target.ID, 10)})
	require.NoError(t, err)
	// Catalog summaries use the same host selectors, without leaking the URL.
	_, err = db.DB.ExecContext(ctx, `UPDATE targets SET last_scrape=? WHERE id=?`, now.UnixMilli(), target.ID)
	require.NoError(t, err)
	hostLabels := map[string]string{"job": "host-fixture", "instance": "host-fixture"}
	node := []model.Sample{}
	for name, value := range map[string]float64{"node_memory_MemTotal_bytes": 1000, "node_memory_MemAvailable_bytes": 400, "node_filesystem_size_bytes": 1000, "node_filesystem_avail_bytes": 750} {
		l := map[string]string{}
		for k, v := range hostLabels {
			l[k] = v
		}
		if name == "node_filesystem_size_bytes" || name == "node_filesystem_avail_bytes" {
			l["device"] = "/dev/fixture"
			l["mountpoint"] = "/"
			l["fstype"] = "ext4"
		}
		node = append(node, model.Sample{Name: name, Labels: l, Value: value, Timestamp: now.UnixMilli()})
	}
	require.NoError(t, db.Ingest(ctx, node))
	w = call(h, "GET", "/api/v1/business/directory", "", "", "")
	require.Equal(t, 200, w.Code, w.Body.String())
	var cards struct {
		Summaries map[string]struct {
			Memory, Disk *float64 `json:"-"`
			M            *float64 `json:"memory_percent"`
			D            *float64 `json:"disk_percent"`
		} `json:"summaries"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &cards))
	require.NotNil(t, cards.Summaries[id].M)
	require.InDelta(t, 60, *cards.Summaries[id].M, 0.01)
	require.NotNil(t, cards.Summaries[id].D)
	require.InDelta(t, 25, *cards.Summaries[id].D, 0.01)
	require.NotContains(t, w.Body.String(), "http://127.0.0.1:19090")
}
