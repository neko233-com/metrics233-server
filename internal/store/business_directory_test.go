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
