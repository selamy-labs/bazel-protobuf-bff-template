package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/server"
)

type options struct {
	addr          string
	allowedOrigin string
	mode          string
	identity      string
}

// parse reads flags, which default to the BFF_* environment.
func parse(args []string, getenv func(string) string) (options, error) {
	var parsed options
	flags := flag.NewFlagSet("bff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&parsed.addr, "addr", fallback(getenv("BFF_ADDR"), defaultAddr(getenv("PORT"))), "listen address")
	flags.StringVar(&parsed.allowedOrigin, "allowed-origin", getenv("BFF_ALLOWED_ORIGIN"), "exact web origin")
	flags.StringVar(&parsed.mode, "mode", fallback(getenv("BFF_MODE"), string(server.ModeProduction)), "only production")
	flags.StringVar(&parsed.identity, "identity", fallback(getenv("BFF_IDENTITY"), "verifier"), "only verifier")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	return parsed, nil
}

// defaultAddr is the listen address when neither the flag nor BFF_ADDR says: every interface on PORT, which a
// hosted container runtime sets (spec 012 FR-018), or 8080 without it. The precedence is the flag, then
// BFF_ADDR, then PORT, then 8080; the ready event prints the address actually bound.
func defaultAddr(port string) string {
	if port == "" {
		return ":8080"
	}
	return "0.0.0.0:" + port
}

func fallback(value, otherwise string) string {
	if value != "" {
		return value
	}
	return otherwise
}

// configure refuses anything but production with a verifier. This binary links no test identity,
// so asking for it is a mistake to report, not a mode to honor.
func configure(parsed options, newVerifier func() (identity.Verifier, error)) (server.Config, error) {
	if parsed.mode != string(server.ModeProduction) {
		return server.Config{}, fmt.Errorf("this binary runs only in production mode, not %q", parsed.mode)
	}
	if parsed.identity != "verifier" {
		return server.Config{}, fmt.Errorf("production mode refuses identity %q", parsed.identity)
	}
	if newVerifier == nil {
		return server.Config{}, errors.New("no verifier is configured; assign newVerifier in bff/cmd/bff/verifier.go")
	}
	verifier, err := newVerifier()
	if err != nil {
		return server.Config{}, fmt.Errorf("verifier: %w", err)
	}
	authenticator, err := identity.NewBearer(verifier)
	if err != nil {
		return server.Config{}, err
	}
	return server.Config{
		Mode:          server.ModeProduction,
		Addr:          parsed.addr,
		AllowedOrigin: parsed.allowedOrigin,
		Authenticator: authenticator,
	}, nil
}
