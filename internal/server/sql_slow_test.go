package server

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestSlowSQLDistanceRedactionAndNamespaceClustering(t *testing.T) {
	require.Equal(t, 3, boundedSQLDistance("kitten", "sitting", 3))
	require.Greater(t, boundedSQLDistance("select", "delete", 1), 1)
	require.NotContains(t, redactSlowSQL("SELECT * FROM Player WHERE password='private-value' AND id=123 /* secret */"), "private")
	require.NotContains(t, redactSlowSQL("SELECT * FROM Player WHERE password='private-value' AND id=123 /* secret */"), "secret")
	require.NotContains(t, redactSlowSQL("SELECT * FROM Player WHERE password='private-value' AND id=123 /* secret */"), "123")
	base := slowSQLGroup{Table: "Player", Namespace: "app/test", Operation: "select", Template: "select * from player where playerid = ? and level > ?", Calls: 2, Seconds: 1}
	near := base
	near.Template = strings.ReplaceAll(base.Template, "level >", "level >=")
	near.Calls = 3
	near.Seconds = 2
	other := base
	other.Table = "Mail"
	scope := base
	scope.Namespace = "app/release"
	groups := clusterSlowSQL([]slowSQLGroup{near, base, scope, other})
	require.Len(t, groups, 3)
	require.EqualValues(t, 5, groups[0].Calls)
	require.EqualValues(t, 3, groups[0].Seconds)
	require.Len(t, groups[0].Variants, 2)
	reversed := clusterSlowSQL([]slowSQLGroup{other, scope, base, near})
	require.Equal(t, groups, reversed)
}
