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

func TestTelemetryProjectIDResolution(t *testing.T) {
	baseEnv := func() map[string]string {
		return map[string]string{
			"BFF_OTEL_ENDPOINT":    "http://127.0.0.1:4318",
			"BFF_RELEASE_REVISION": strings.Repeat("a", 40),
		}
	}

	t.Run("valid GCP project ID is wired", func(t *testing.T) {
		env := baseEnv()
		env["GOOGLE_CLOUD_PROJECT"] = "example-gcp-project"
		c, sources, err := resolveTelemetry(func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		if c.ProjectID != "example-gcp-project" {
			t.Errorf("ProjectID = %q, want %q", c.ProjectID, "example-gcp-project")
		}
		if sources["GOOGLE_CLOUD_PROJECT"] != "environment" {
			t.Errorf("sources[GOOGLE_CLOUD_PROJECT] = %q, want environment", sources["GOOGLE_CLOUD_PROJECT"])
		}
	})

	t.Run("valid GCP project ID with surrounding whitespace is trimmed", func(t *testing.T) {
		env := baseEnv()
		env["GOOGLE_CLOUD_PROJECT"] = "  my-prod-project-123  "
		c, sources, err := resolveTelemetry(func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		if c.ProjectID != "my-prod-project-123" {
			t.Errorf("ProjectID = %q, want %q", c.ProjectID, "my-prod-project-123")
		}
		if sources["GOOGLE_CLOUD_PROJECT"] != "environment" {
			t.Errorf("sources[GOOGLE_CLOUD_PROJECT] = %q, want environment", sources["GOOGLE_CLOUD_PROJECT"])
		}
	})

	t.Run("invalid GCP project ID leaves ProjectID empty for bare trace ID emission", func(t *testing.T) {
		for _, invalid := range []string{
			"INVALID_UPPERCASE",
			"123-leading-digit",
			"-leading-dash",
			"trailing-dash-",
			"shrt",
			strings.Repeat("a", 31),
			"invalid_char$",
		} {
			env := baseEnv()
			env["GOOGLE_CLOUD_PROJECT"] = invalid
			c, sources, err := resolveTelemetry(func(k string) string { return env[k] })
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", invalid, err)
			}
			if c.ProjectID != "" {
				t.Errorf("expected empty ProjectID for invalid %q, got %q", invalid, c.ProjectID)
			}
			if sources["GOOGLE_CLOUD_PROJECT"] != "environment (ignored: invalid)" {
				t.Errorf("expected 'environment (ignored: invalid)' source for invalid %q, got %q", invalid, sources["GOOGLE_CLOUD_PROJECT"])
			}
		}
	})

	t.Run("absent GOOGLE_CLOUD_PROJECT leaves ProjectID empty", func(t *testing.T) {
		env := baseEnv()
		c, sources, err := resolveTelemetry(func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		if c.ProjectID != "" {
			t.Errorf("expected empty ProjectID, got %q", c.ProjectID)
		}
		if sources["GOOGLE_CLOUD_PROJECT"] != "composition" {
			t.Errorf("expected composition source, got %q", sources["GOOGLE_CLOUD_PROJECT"])
		}
	})
}
