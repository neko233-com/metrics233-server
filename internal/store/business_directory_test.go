package store

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/metrics233-server/internal/model"
	"github.com/stretchr/testify/require"
)

func TestBusinessDirectoryGlobalFiltersNaturalPagingAndNoteCAS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.db")
	s, err := Open(path)
	require.NoError(t, err)
	defer s.DB.Close()
	ctx := context.Background()
	samples := []model.Sample{}
	for i := 0; i < 75; i++ {
		env, role := "release", "logic"
		if i%2 == 0 {
			env = "test"
		}
		if i >= 70 {
			role = "sdk"
		}
		samples = append(samples, model.Sample{Name: "fixture", Value: 0, Labels: map[string]string{"service": "fixture", "role": role, "serverId": fmt.Sprint(i), "instance": fmt.Sprint("instance-", i), "environment": env}})
	}
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, "192.0.2.1"))
	filter := BusinessDirectoryFilter{Limit: 24, Sort: "server"}
	first, err := s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 75, first.Total)
	require.Equal(t, 70, first.Counts["game"])
	require.Equal(t, 5, first.Counts["sdk"])
	require.Equal(t, "0", first.Items[0].Identity.ServerID)
	require.Equal(t, "23", first.Items[23].Identity.ServerID)
	filter.Cursor = first.Next
	second, err := s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, "24", second.Items[0].Identity.ServerID)
	filter.Environment = "test"
	_, err = s.BusinessDirectory(ctx, filter)
	require.Error(t, err, "a cursor cannot be reused across filters")
	filter.Cursor = ""
	filter.Role = "sdk"
	page, err := s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 3, page.Matched)
	require.Equal(t, 38, page.Counts["all"])
	id := page.Items[0].ID
	note, err := s.SaveBusinessNote(ctx, id, model.BusinessNote{Description: "微信正式服\n观察网络波动 <script>alert(1)</script>"})
	require.NoError(t, err)
	require.EqualValues(t, 1, note.Version)
	_, err = s.SaveBusinessNote(ctx, id, model.BusinessNote{Version: 0, Description: "lost update"})
	require.ErrorIs(t, err, ErrBusinessNoteConflict)
	_, err = s.SaveBusinessNote(ctx, id, model.BusinessNote{Version: 1, Description: strings.Repeat("中", 1366)})
	require.Error(t, err)
	_, err = s.SaveBusinessNote(ctx, id, model.BusinessNote{Version: 1, Description: "bad\x00text"})
	require.Error(t, err)
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, "192.0.2.2"))
	filter.Query = "网络波动"
	page, err = s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, note, page.Items[0].Note)
	require.NoError(t, s.Close())
	s, err = Open(path)
	require.NoError(t, err)
	defer s.Close()
	item, err := s.BusinessInstance(ctx, id)
	require.NoError(t, err)
	require.Equal(t, note, item.Note)
}

func TestBusinessDirectoryPlatformFacetsAndPaging(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "catalog.db"))
	require.NoError(t, err)
	defer s.Close()
	ctx := context.Background()
	var samples []model.Sample
	add := func(id, channel, role, environment string) {
		samples = append(samples, model.Sample{Name: "fixture", Labels: map[string]string{
			"service": "fixture", "role": role, "serverId": id, "instance": "host-" + id,
			"environment": environment, "channel": channel,
		}})
	}
	for i := 1; i <= 30; i++ {
		add(fmt.Sprint(i), "minigame-wechat", "logic", "test")
	}
	add("31", "WX", "center", "test")
	add("32", "TapTap", "logic", "release")
	add("33", "studio-beta", "logic", "release")
	add("34", "custom:studio-beta", "logic", "release")
	add("35", "", "gateway", "release")
	add("36", "android-default", "logic", "audit")
	require.NoError(t, s.RecordBusinessInstances(ctx, samples, "192.0.2.1"))

	filter := BusinessDirectoryFilter{Limit: 24}
	first, err := s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, map[string]int{
		"wechat": 31, "taptap": 1, "custom:studio-beta": 1,
		"custom:custom:studio-beta": 1, "unassigned": 1, "android": 1,
	}, first.Platforms)
	filter.Platform = "taptap"
	filter.Cursor = first.Next
	_, err = s.BusinessDirectory(ctx, filter)
	require.ErrorContains(t, err, "cursor does not match filters")
	filter.Cursor = ""
	page, err := s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "32", page.Items[0].Identity.ServerID)
	require.Equal(t, "TapTap", page.Items[0].Identity.Channel)
	require.Equal(t, "taptap", page.Items[0].Platform)

	filter.Platform = "wechat"
	filter.Environment = "test"
	filter.Role = "center"
	page, err = s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Equal(t, 1, page.Platforms["wechat"])
	require.Equal(t, 0, page.Platforms["taptap"])
	require.Equal(t, 30, page.Counts["game"])
	require.Len(t, page.Items, 1)
	require.Equal(t, "center", page.Items[0].Identity.Role)

	filter = BusinessDirectoryFilter{Limit: 24, Platform: "custom:studio-beta", Query: "host-33"}
	page, err = s.BusinessDirectory(ctx, filter)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, "33", page.Items[0].Identity.ServerID)
}
