package server

import (
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
	"math"
	"testing"
)

func TestNetworkRecommendationUsesFreshTrafficAndPhysicalTieBreak(t *testing.T) {
	devices := []string{"anpi0", "bridge0", "en0", "en1", "utun0"}
	for _, fixture := range []struct {
		activity map[string]float64
		want     string
	}{
		{nil, "en0"},
		{map[string]float64{"bridge0": 1000, "en0": 10, "en1": 20}, "en1"},
		{map[string]float64{"en0": 0, "utun0": 20}, "utun0"},
		{map[string]float64{"en0": math.NaN(), "en1": math.Inf(1), "bridge0": 1}, "bridge0"},
		{map[string]float64{"en0": 20, "en1": 20}, "en0"},
	} {
		require.Equal(t, fixture.want, defaultNetworkDevice(devices, fixture.activity))
	}
	require.Empty(t, defaultNetworkDevice(nil, nil))
}

func TestBusinessTrafficExpressionsParse(t *testing.T) {
	for _, role := range []string{"logic", "game", "gateway", "sdk"} {
		for _, direction := range []string{"receive", "transmit"} {
			for _, operation := range []string{"rate", "increase"} {
				expr := businessTrafficExpr(role, direction, `{role="`+role+`"}`, "2m", operation)
				_, err := parser.NewParser(parser.Options{}).ParseExpr(expr)
				require.NoError(t, err, expr)
			}
		}
	}
}
