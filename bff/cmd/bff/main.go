// Command bff is the production server. As shipped it refuses to start, because this repository
// includes no identity provider: assign newVerifier in verifier.go first.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/bfftemplate/bff/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	options, err := parse(os.Args[1:], os.Getenv)
	if err == nil {
		var config server.Config
		config, err = configure(options, newVerifier)
		if err == nil {
			config.Logger = slog.New(slog.NewJSONHandler(os.Stderr, nil))
			config.Stdout = os.Stdout
			err = server.Run(ctx, config)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "bff: %v\n", err)
		stop()
		os.Exit(1)
	}
}
