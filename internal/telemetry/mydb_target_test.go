package telemetry

import "testing"

func TestMyDBCollectionRequiresHTTPSOutsideLoopback(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"http://127.0.0.1:4306/metrics", true},
		{"http://[::1]:4306/metrics", true},
		{"http://localhost:4306/metrics", true},
		{"https://db-metrics.example.com/metrics", true},
		{"https://db-metrics.example.com/metrics?token=private", false},
		{"https://db-metrics.example.com/metrics?", false},
		{"http://db-metrics.example.com/metrics", false},
		{"http://192.0.2.1:4306/metrics", false},
		{"http://127.0.0.1.example.com/metrics", false},
		{"http://localhost.example.com/metrics", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			target := Target{Name: "mydb", Kind: "mydb", URL: tc.url, IntervalSeconds: 15}
			if err := target.Validate(); (err == nil) != tc.valid {
				t.Fatalf("valid=%t, error=%v", tc.valid, err)
			}
		})
	}
}
