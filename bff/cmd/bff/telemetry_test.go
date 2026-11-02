package main

import (
	"strings"
	"testing"
	"time"
)

func TestTelemetryCompositionDefaultsAndProvenance(t *testing.T) {
	env := map[string]string{"BFF_OTEL_ENDPOINT": "http://127.0.0.1:4318", "BFF_RELEASE_REVISION": strings.Repeat("a", 40)}
	c, sources, err := resolveTelemetry(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.QueueSize != 2048 || c.BatchSize != 512 || *c.SampleRatio != 1 || sources["BFF_OTEL_ENDPOINT"] != "environment" || sources["BFF_OTEL_QUEUE_SIZE"] != "composition" {
		t.Fatalf("config=%+v sources=%v", c, sources)
	}
	env["BFF_OTEL_QUEUE_SIZE"] = "8"
	env["BFF_OTEL_BATCH_SIZE"] = "2"
	env["BFF_OTEL_EXPORT_TIMEOUT"] = "200ms"
	env["BFF_OTEL_SAMPLE_RATIO"] = "0"
	c, sources, err = resolveTelemetry(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.QueueSize != 8 || c.BatchSize != 2 || c.ExportTimeout != 200*time.Millisecond || *c.SampleRatio != 0 || sources["BFF_OTEL_EXPORT_TIMEOUT"] != "environment" {
		t.Fatalf("config=%+v sources=%v", c, sources)
	}
}
func TestTelemetryCompositionRefusesMissingAndInvalid(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"BFF_OTEL_ENDPOINT", ""}, {"BFF_RELEASE_REVISION", ""}, {"BFF_OTEL_SAMPLE_RATIO", "NaN"}, {"BFF_OTEL_QUEUE_SIZE", "many"}, {"BFF_OTEL_BATCH_DELAY", "forever"}, {"BFF_OTEL_BUCKETS", ".1,.1"}} {
		env := map[string]string{"BFF_OTEL_ENDPOINT": "http://127.0.0.1:4318", "BFF_RELEASE_REVISION": strings.Repeat("a", 40)}
		env[tc.key] = tc.value
		if _, _, err := resolveTelemetry(func(k string) string { return env[k] }); err == nil {
			t.Errorf("accepted %s=%q", tc.key, tc.value)
		}
	}
}
