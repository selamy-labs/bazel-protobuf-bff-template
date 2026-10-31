// Command bff-dev is the local development server. It accepts any test-identity subject and injects
// faults, so it is testonly, listens only on loopback, and says what it is when it starts.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/server"
	"example.com/bfftemplate/bff/testing/faults"
	testidentity "example.com/bfftemplate/bff/testing/identity"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "loopback listen address")
	origin := flag.String("allowed-origin", "http://127.0.0.1:5173", "exact web origin")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(os.Stderr, "bff-dev: TEST IDENTITY: any caller can claim any subject; never deploy this binary")
	err := server.Run(ctx, server.Config{
		Mode:          server.ModeDev,
		Addr:          *addr,
		AllowedOrigin: *origin,
		Authenticator: testidentity.Authenticator{},
		Store:         faults.Store{Next: memory.New()},
		Logger:        slog.New(slog.NewJSONHandler(os.Stderr, nil)),
		Stdout:        os.Stdout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bff-dev: %v\n", err)
		stop()
		os.Exit(1)
	}
}
