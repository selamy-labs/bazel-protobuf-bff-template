// Package server wires the application into an HTTP server. Both binaries use it, so the dev binary
// exercises the same code as production. Run refuses a configuration that would weaken identity.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"example.com/bfftemplate/bff/app"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/ports"
	"example.com/bfftemplate/bff/transport"
)

// Mode selects what a server may accept.
type Mode string

const (
	// ModeProduction requires verified identity.
	ModeProduction Mode = "production"
	// ModeDev allows test identity on a loopback address.
	ModeDev Mode = "dev"
)

const shutdownBound = 5 * time.Second

// Config is everything a server needs.
type Config struct {
	Mode          Mode
	Addr          string
	AllowedOrigin string
	Authenticator identity.Authenticator
	Store         ports.Store
	Logger        *slog.Logger
	Stdout        io.Writer
}

// Check refuses a configuration before anything is served.
func (c Config) Check() error {
	if c.Authenticator == nil {
		return errors.New("no authenticator is configured")
	}
	if c.Store == nil {
		return errors.New("no store is configured")
	}
	if err := checkOrigin(c.AllowedOrigin); err != nil {
		return err
	}
	switch c.Mode {
	case ModeProduction:
		if c.Authenticator.Kind() != identity.KindVerified {
			return fmt.Errorf("production mode refuses %s", c.Authenticator.Kind())
		}
		return nil
	case ModeDev:
		if c.Authenticator.Kind() != identity.KindTestIdentity {
			return errors.New("dev mode expects test identity")
		}
		return checkLoopback(c.Addr)
	default:
		return fmt.Errorf("unknown mode %q", c.Mode)
	}
}

func checkOrigin(origin string) error {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.Path != "" || parsed.RawQuery != "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("allowed origin %q must be an exact scheme://host[:port]", origin)
	}
	return nil
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("dev mode must listen on a loopback IP address, not %q", addr)
	}
	return nil
}

// NewHandler returns the handler for a checked configuration.
func NewHandler(config Config) http.Handler {
	return transport.NewHandler(transport.Options{
		Notes:             app.New(config.Store),
		Authenticator:     config.Authenticator,
		AllowTestIdentity: config.Mode == ModeDev,
		AllowedOrigin:     config.AllowedOrigin,
		Logger:            config.Logger,
	})
}

type readyEvent struct {
	Event    string `json:"event"`
	Addr     string `json:"addr"`
	Mode     Mode   `json:"mode"`
	Identity string `json:"identity"`
}

// Run serves until ctx ends, then drains for a bounded time. It prints one ready event once the
// listener is open.
func Run(ctx context.Context, config Config) error {
	if err := config.Check(); err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", config.Addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler:           NewHandler(config),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      transport.MaxDeadline + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	served := make(chan error, 1)
	go func() { served <- server.Serve(listener) }()

	ready, err := json.Marshal(readyEvent{"ready", listener.Addr().String(), config.Mode, config.Authenticator.Kind().String()})
	if err != nil {
		return errors.Join(err, server.Close())
	}
	if _, err := fmt.Fprintln(config.Stdout, string(ready)); err != nil {
		return errors.Join(err, server.Close())
	}

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	drain, cancel := context.WithTimeout(context.Background(), shutdownBound)
	defer cancel()
	if err := server.Shutdown(drain); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
}
