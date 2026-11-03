package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"example.com/bfftemplate/bff/server"
	"example.com/bfftemplate/bff/telemetry"
)

var gcpProjectID = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

// resolveTelemetry is the single default site. Environment overrides composition;
// endpoint and revision have no default. The returned map records every source.
func resolveTelemetry(getenv func(string) string) (telemetry.Config, map[string]string, error) {
	sources := map[string]string{"version": "contract-v1", "service_name": "contract-v1"}
	read := func(name, otherwise string) string {
		value := getenv(name)
		sources[name] = "environment"
		if value == "" {
			value = otherwise
			sources[name] = "composition"
		}
		return value
	}
	c := telemetry.Config{Version: 1, ServiceName: "bff-server", Endpoint: read("BFF_OTEL_ENDPOINT", ""), ServiceVersion: read("BFF_RELEASE_REVISION", ""), Environment: read("BFF_OTEL_ENVIRONMENT", "prod")}

	rawProjectID := getenv("GOOGLE_CLOUD_PROJECT")
	if rawProjectID != "" {
		projectID := strings.TrimSpace(rawProjectID)
		if gcpProjectID.MatchString(projectID) {
			c.ProjectID = projectID
			sources["GOOGLE_CLOUD_PROJECT"] = "environment"
		} else {
			sources["GOOGLE_CLOUD_PROJECT"] = "environment (ignored: invalid)"
		}
	} else {
		sources["GOOGLE_CLOUD_PROJECT"] = "composition"
	}

	ratio, err := strconv.ParseFloat(read("BFF_OTEL_SAMPLE_RATIO", "1"), 64)
	if err != nil {
		return c, sources, errors.New("BFF_OTEL_SAMPLE_RATIO: number required")
	}
	c.SampleRatio = &ratio
	for _, field := range []struct {
		name, value string
		target      *int
	}{{"BFF_OTEL_QUEUE_SIZE", "2048", &c.QueueSize}, {"BFF_OTEL_BATCH_SIZE", "512", &c.BatchSize}} {
		*field.target, err = strconv.Atoi(read(field.name, field.value))
		if err != nil {
			return c, sources, fmt.Errorf("%s: integer required", field.name)
		}
	}
	for _, field := range []struct {
		name, value string
		target      *time.Duration
	}{{"BFF_OTEL_BATCH_DELAY", "1s", &c.BatchDelay}, {"BFF_OTEL_EXPORT_TIMEOUT", "2s", &c.ExportTimeout}, {"BFF_OTEL_METRIC_INTERVAL", "10s", &c.MetricInterval}, {"BFF_OTEL_SHUTDOWN_TIMEOUT", "5s", &c.ShutdownTimeout}} {
		*field.target, err = time.ParseDuration(read(field.name, field.value))
		if err != nil {
			return c, sources, fmt.Errorf("%s: duration required", field.name)
		}
	}
	c.Buckets, err = parseBuckets(read("BFF_OTEL_BUCKETS", "0.001,0.005,0.01,0.05,0.1,0.5,1,5"))
	if err != nil {
		return c, sources, err
	}
	return c, sources, c.Check()
}

// runInstrumented owns telemetry for exactly one server lifetime. Configure the
// stdout/stderr sinks before calling. An existing base logger is intentionally
// replaced: raw downstream errors must pass through the restrictive bridge.
func runInstrumented(ctx context.Context, config server.Config, c telemetry.Config, sink slog.Handler) error {
	runtime, err := telemetry.New(ctx, c)
	if err != nil {
		return err
	}
	config.Store, err = runtime.DecorateStore(config.Store)
	if err == nil {
		config.Telemetry, err = runtime.Interceptor()
	}
	if err == nil {
		config.Logger = slog.New(runtime.LogHandler(sink))
		err = server.Run(ctx, config)
	}
	// signal context is already canceled here; use a fresh bounded drain context.
	return errors.Join(err, runtime.Shutdown(context.Background()))
}

func parseBuckets(raw string) ([]float64, error) {
	var buckets []float64
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, errors.New("BFF_OTEL_BUCKETS: comma-separated numbers required")
		}
		buckets = append(buckets, value)
	}
	return buckets, nil
}
