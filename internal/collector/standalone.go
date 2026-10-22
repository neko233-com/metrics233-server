package collector

import (
	"context"
	"net/http"
	"time"

	model "github.com/neko233-com/metrics233-server/internal/telemetry"
)

// CollectOnce performs the same bounded read-only collection without a local
// database or HTTP listener. It never modifies the caller's target or secrets.
func CollectOnce(ctx context.Context, target model.Target, secrets map[string]string) ([]model.Sample, error) {
	if err := target.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	c := &Collector{Client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
	return c.collect(ctx, target, secrets)
}
