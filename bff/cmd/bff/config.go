package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"time"

	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/server"
)

type options struct {
	addr            string
	allowedOrigin   string
	mode            string
	identity        string
	defaultDeadline time.Duration
	maxDeadline     time.Duration
	maxRequestBytes int
	shutdownBound   time.Duration
}

// The composition's defaults (spec 011 FR-016): each is set here once, read by its flag, then its BFF_*
// variable, then this value; nothing below this package carries a default of its own.
const (
	defaultDeadline = 2 * time.Second
	maxDeadline     = 10 * time.Second
	maxRequestBytes = 64 << 10
	shutdownBound   = 5 * time.Second
)

// The listener's bounds, set once here and not read from outside: the server requires them.
var listener = server.ListenerLimits{
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       15 * time.Second,
	IdleTimeout:       60 * time.Second,
	MaxHeaderBytes:    16 << 10,
}

// parse reads flags, which default to the BFF_* environment. A variable that does not parse is refused by name.
func parse(args []string, getenv func(string) string) (options, error) {
	var parsed options
	flags := flag.NewFlagSet("bff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&parsed.addr, "addr", fallback(getenv("BFF_ADDR"), defaultAddr(getenv("PORT"))), "listen address")
	flags.StringVar(&parsed.allowedOrigin, "allowed-origin", getenv("BFF_ALLOWED_ORIGIN"), "exact web origin")
	flags.StringVar(&parsed.mode, "mode", fallback(getenv("BFF_MODE"), string(server.ModeProduction)), "only production")
	flags.StringVar(&parsed.identity, "identity", fallback(getenv("BFF_IDENTITY"), "verifier"), "only verifier")
	durations := []struct {
		target *time.Duration
		flag   string
		env    string
		value  time.Duration
		usage  string
	}{
		{&parsed.defaultDeadline, "default-deadline", "BFF_DEFAULT_DEADLINE", defaultDeadline, "deadline for a call that sets none"},
		{&parsed.maxDeadline, "max-deadline", "BFF_MAX_DEADLINE", maxDeadline, "cap on any client deadline"},
		{&parsed.shutdownBound, "shutdown-bound", "BFF_SHUTDOWN_BOUND", shutdownBound, "how long a stop drains"},
	}
	for _, d := range durations {
		value, err := envDuration(getenv, d.env, d.value)
		if err != nil {
			return options{}, err
		}
		flags.DurationVar(d.target, d.flag, value, d.usage)
	}
	bytes, err := envInt(getenv, "BFF_MAX_REQUEST_BYTES", maxRequestBytes)
	if err != nil {
		return options{}, err
	}
	flags.IntVar(&parsed.maxRequestBytes, "max-request-bytes", bytes, "largest request body, enforced before decoding")
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

func envDuration(getenv func(string) string, name string, otherwise time.Duration) (time.Duration, error) {
	raw := getenv(name)
	if raw == "" {
		return otherwise, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a duration such as 2s or 500ms", name, raw)
	}
	return value, nil
}

func envInt(getenv func(string) string, name string, otherwise int) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return otherwise, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: %q is not a whole number", name, raw)
	}
	return value, nil
}

// configure refuses anything but production with a verifier. This binary links no test identity,
// so asking for it is a mistake to report, not a mode to honor. The bounds go down as required
// parameters; the server's check refuses a bound outside its invariants, whatever asked for it.
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
	limits := listener
	limits.ShutdownBound = parsed.shutdownBound
	return server.Config{
		Mode:          server.ModeProduction,
		Addr:          parsed.addr,
		AllowedOrigin: parsed.allowedOrigin,
		Authenticator: authenticator,
		Calls: server.CallLimits{
			DefaultDeadline: parsed.defaultDeadline,
			MaxDeadline:     parsed.maxDeadline,
			MaxRequestBytes: parsed.maxRequestBytes,
		},
		Listener: limits,
	}, nil
}
