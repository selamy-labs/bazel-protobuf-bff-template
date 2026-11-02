// Command bff is the production server. Issuer and telemetry inputs are required at composition.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/bfftemplate/bff/adapters/identity/oidc"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/server"
	"example.com/bfftemplate/bff/telemetry"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	err := launch(ctx)
	stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bff: %v\n", err)
		os.Exit(1)
	}
}
func launch(ctx context.Context) error {
	options, err := parse(os.Args[1:], os.Getenv)
	if err != nil {
		return err
	}
	config, err := configure(options, func(s oidc.Settings) (identity.Verifier, error) { return newVerifier(ctx, s) })
	if err != nil {
		return err
	}
	return launchInstrumented(ctx, config)
}
func launchInstrumented(ctx context.Context, config server.Config) error {
	telemetryConfig, sources, err := resolveTelemetry(os.Getenv)
	if err != nil {
		return err
	}
	config.Stdout = os.Stdout
	err = json.NewEncoder(os.Stderr).Encode(struct {
		Event   string
		Config  telemetry.Config
		Sources map[string]string
	}{"telemetry_config", telemetryConfig, sources})
	if err != nil {
		return err
	}
	return runInstrumented(ctx, config, telemetryConfig, slog.NewJSONHandler(os.Stderr, nil))
}
