package server

import (
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
	"testing"
)

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
