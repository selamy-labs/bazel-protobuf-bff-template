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
	"time"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/adapters/store/local"
	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/server"
	"example.com/bfftemplate/bff/testing/faults"
	testidentity "example.com/bfftemplate/bff/testing/identity"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "loopback listen address")
	origin := flag.String("allowed-origin", "http://127.0.0.1:5173", "exact web origin")
	storeKind := flag.String("store", "memory", "memory, or local with --data-dir")
	dataDir := flag.String("data-dir", "", "the local store's directory")
	flag.Parse()
	store, err := openStore(*storeKind, *dataDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bff-dev: %v\n", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(os.Stderr, "bff-dev: TEST IDENTITY: any caller can claim any subject; never deploy this binary")
	err = server.Run(ctx, server.Config{
		Mode:          server.ModeDev,
		Addr:          *addr,
		AllowedOrigin: *origin,
		Authenticator: testidentity.Authenticator{},
		Store:         faults.Store{Next: store},
		Logger:        slog.New(slog.NewJSONHandler(os.Stderr, nil)),
		Stdout:        os.Stdout,
		// The local demo's bounds, declared once here (spec 011 FR-016); the server carries none.
		Calls:    server.CallLimits{DefaultDeadline: 2 * time.Second, MaxDeadline: 10 * time.Second, MaxRequestBytes: 64 << 10},
		Listener: server.ListenerLimits{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, ShutdownBound: 5 * time.Second},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "bff-dev: %v\n", err)
		stop()
		os.Exit(1)
	}
}

// openStore is the one place the local demo's persistence is chosen (spec 012 FR-022).
func openStore(kind, dir string) (ports.Store, error) {
	switch kind {
	case "memory":
		return memory.New(10000), nil // the local demo's bound, declared once here
	case "local":
		if dir == "" {
			return nil, fmt.Errorf("--store=local needs --data-dir")
		}
		return local.Open(dir)
	default:
		return nil, fmt.Errorf("--store=%q is neither memory nor local", kind)
	}
}
