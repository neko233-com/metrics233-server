package collector

import (
	"context"
	"time"

	model "github.com/neko233-com/metrics233-server/internal/telemetry"
)

// Preview reads through the actual bounded protocol implementation and slots,
// but does not ingest samples, save a target, or alter scrape health.
func (c *Collector) Preview(ctx context.Context, target model.Target, secrets map[string]string) ([]model.Sample, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return c.collect(ctx, target, secrets)
}
