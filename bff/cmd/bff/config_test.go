package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/bfftemplate/bff/adapters/identity/oidc"
	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/server"
)

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) {
	return "", identity.ErrUnauthenticated
}

func withVerifier(oidc.Settings) (identity.Verifier, error) { return verifier{}, nil }

func noEnvironment(string) string { return "" }

// The shipped binary names no issuer, so it refuses to start: the refusal spec 009's image witnesses.
func TestShippedBinaryRefusesWithoutAnIssuer(t *testing.T) {
	parsed, err := parse([]string{"--allowed-origin=https://app.example.com"}, noEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configure(parsed, func(s oidc.Settings) (identity.Verifier, error) { return newVerifier(t.Context(), s) }); err == nil || !strings.Contains(err.Error(), "no verifier is configured") {
		t.Fatalf("got %v", err)
	}
}

// The issuer's three settings arrive by flag or variable and reach the verifier; a partial set is refused by name.
func TestTheIssuerIsTheComposition(t *testing.T) {
	var got oidc.Settings
	capture := func(s oidc.Settings) (identity.Verifier, error) { got = s; return verifier{}, nil }
	parsed, err := parse([]string{"--verifier-issuer=https://issuer.example/", "--verifier-audience=bff"}, func(name string) string {
		return map[string]string{"BFF_VERIFIER_JWKS_URL": "https://issuer.example/jwks"}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configure(parsed, capture); err != nil {
		t.Fatal(err)
	}
	want := oidc.Settings{Issuer: "https://issuer.example/", Audience: "bff", JWKSURL: "https://issuer.example/jwks", Policy: verifierPolicy()}
	if got != want {
		t.Fatalf("the verifier got %+v, want %+v", got, want)
	}
	parsed, err = parse([]string{"--verifier-issuer=https://issuer.example/"}, noEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := configure(parsed, func(s oidc.Settings) (identity.Verifier, error) { return newVerifier(t.Context(), s) }); err == nil || !strings.Contains(err.Error(), "verifier.audience: required") {
		t.Fatalf("a partial issuer: got %v", err)
	}
}

// The listen address at the funnel (spec 012 FR-018): the flag, then BFF_ADDR, then PORT, then 8080.
func TestListenAddressPrecedence(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	cases := map[string]struct {
		args []string
		env  map[string]string
		want string
	}{
		"nothing":                  {nil, nil, ":8080"},
		"PORT alone":               {nil, map[string]string{"PORT": "9090"}, "0.0.0.0:9090"},
		"BFF_ADDR over PORT":       {nil, map[string]string{"PORT": "9090", "BFF_ADDR": "127.0.0.1:7070"}, "127.0.0.1:7070"},
		"the flag over everything": {[]string{"--addr=:6060"}, map[string]string{"PORT": "9090", "BFF_ADDR": ":7070"}, ":6060"},
	}
	for name, c := range cases {
		parsed, err := parse(c.args, env(c.env))
		if err != nil || parsed.addr != c.want {
			t.Errorf("%s: addr %q, err %v; want %q", name, parsed.addr, err, c.want)
		}
	}
}

func TestConfigure(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		env      map[string]string
		verifier func(oidc.Settings) (identity.Verifier, error)
		want     string
	}{
		{name: "no verifier", want: "no verifier is configured"},
		{name: "a failing verifier", verifier: func(oidc.Settings) (identity.Verifier, error) { return nil, errors.New("no key") }, want: "no key"},
		{name: "a nil verifier", verifier: func(oidc.Settings) (identity.Verifier, error) { return nil, nil }, want: "no verifier is configured"},
		{name: "test identity by flag", args: []string{"--identity=test-identity"}, verifier: withVerifier, want: "refuses identity"},
		{name: "test identity by environment", env: map[string]string{"BFF_IDENTITY": "test-identity"}, verifier: withVerifier, want: "refuses identity"},
		{name: "dev mode by flag", args: []string{"--mode=dev"}, verifier: withVerifier, want: "only in production"},
		{name: "dev mode by environment", env: map[string]string{"BFF_MODE": "dev"}, verifier: withVerifier, want: "only in production"},
		{name: "with a verifier", verifier: withVerifier},
		{name: "no notes at all", args: []string{"--max-notes=0"}, verifier: withVerifier, want: "max-notes: 0 is outside"},
		{name: "notes beyond the ceiling", env: map[string]string{"BFF_MAX_NOTES": "5000000"}, verifier: withVerifier, want: "max-notes: 5000000 is outside"},
		{name: "notes from the environment", env: map[string]string{"BFF_MAX_NOTES": "50"}, verifier: withVerifier},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Every case names an issuer, so that only the case's own concern decides it.
			args := append([]string{"--verifier-issuer=https://issuer.example/", "--verifier-audience=bff", "--verifier-jwks-url=https://issuer.example/jwks"}, c.args...)
			parsed, err := parse(args, func(key string) string { return c.env[key] })
			if err != nil {
				t.Fatal(err)
			}
			config, err := configure(parsed, c.verifier)
			if c.want != "" {
				if err == nil || !strings.Contains(err.Error(), c.want) {
					t.Fatalf("want %q, got %v", c.want, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if config.Mode != server.ModeProduction || config.Authenticator.Kind() != identity.KindVerified || config.Store == nil {
				t.Fatalf("unexpected config %+v", config)
			}
		})
	}
}

func TestConfiguredProductionPassesTheServerCheck(t *testing.T) {
	parsed, err := parse([]string{"--allowed-origin=https://app.example.com", "--verifier-issuer=https://issuer.example/", "--verifier-audience=bff", "--verifier-jwks-url=https://issuer.example/jwks"}, noEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	config, err := configure(parsed, withVerifier)
	if err != nil {
		t.Fatal(err)
	}
	config.Store = memory.New(100)
	if err := config.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownFlagIsAnError(t *testing.T) {
	if _, err := parse([]string{"--no-such-flag"}, noEnvironment); err == nil {
		t.Fatal("want an error")
	}
}

// Each bound: the flag, then its BFF_* variable, then the composition's value (spec 011 FR-016).
func TestBoundsPrecedence(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	cases := map[string]struct {
		args  []string
		env   map[string]string
		max   time.Duration
		bytes int
	}{
		"nothing: the composition's values": {nil, nil, 10 * time.Second, 64 << 10},
		"the environment over the values":   {nil, map[string]string{"BFF_MAX_DEADLINE": "4s", "BFF_MAX_REQUEST_BYTES": "16384"}, 4 * time.Second, 16384},
		"the flag over the environment":     {[]string{"--max-deadline=3s", "--max-request-bytes=32768"}, map[string]string{"BFF_MAX_DEADLINE": "4s", "BFF_MAX_REQUEST_BYTES": "16384"}, 3 * time.Second, 32768},
	}
	for name, c := range cases {
		parsed, err := parse(c.args, env(c.env))
		if err != nil || parsed.maxDeadline != c.max || parsed.maxRequestBytes != c.bytes {
			t.Errorf("%s: %+v, %v", name, parsed, err)
		}
	}
	for name, values := range map[string]map[string]string{
		"BFF_MAX_DEADLINE":      {"BFF_MAX_DEADLINE": "soon"},
		"BFF_DEFAULT_DEADLINE":  {"BFF_DEFAULT_DEADLINE": "2"},
		"BFF_SHUTDOWN_BOUND":    {"BFF_SHUTDOWN_BOUND": "5 seconds"},
		"BFF_MAX_REQUEST_BYTES": {"BFF_MAX_REQUEST_BYTES": "64k"},
	} {
		_, err := parse(nil, env(values))
		if err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: got %v, want a refusal naming it", name, err)
		}
	}
}

// A configuration that would weaken an invariant is refused by the server's check, whatever source gave it.
func TestABoundBeyondTheInvariantIsRefused(t *testing.T) {
	for name, args := range map[string][]string{
		"the maximum deadline beyond its ceiling": {"--max-deadline=5m"},
		"the default above the maximum":           {"--default-deadline=20s"},
		"a request bound beyond its ceiling":      {"--max-request-bytes=10485760"},
		"a drain beyond its ceiling":              {"--shutdown-bound=10m"},
	} {
		parsed, err := parse(append([]string{"--allowed-origin=https://app.example.com", "--verifier-issuer=https://issuer.example/", "--verifier-audience=bff", "--verifier-jwks-url=https://issuer.example/jwks"}, args...), noEnvironment)
		if err != nil {
			t.Fatal(err)
		}
		config, err := configure(parsed, withVerifier)
		if err != nil {
			t.Fatal(err)
		}
		config.Store = memory.New(100)
		if err := config.Check(); err == nil || !strings.Contains(err.Error(), "ceiling") && !strings.Contains(err.Error(), "exceeds") {
			t.Errorf("%s: got %v", name, err)
		}
	}
}

// The store is the composition's choice (spec 012 FR-017): memory by default, the object store with its bucket, and
// nothing else; the bucket is required with the object store.
func TestTheStoreIsTheComposition(t *testing.T) {
	issuer := []string{"--allowed-origin=https://app.example.com", "--verifier-issuer=https://issuer.example/", "--verifier-audience=bff", "--verifier-jwks-url=https://issuer.example/jwks"}
	cases := map[string]struct {
		args []string
		env  map[string]string
		want string
	}{
		"memory by the composition":   {nil, nil, ""},
		"object by flag":              {[]string{"--store=object", "--object-bucket=notes"}, nil, ""},
		"object by the environment":   {nil, map[string]string{"BFF_STORE": "object", "BFF_OBJECT_BUCKET": "notes"}, ""},
		"object without a bucket":     {[]string{"--store=object"}, nil, "object-bucket: required"},
		"a store that does not exist": {[]string{"--store=cloud"}, nil, "neither memory nor object"},
	}
	for name, c := range cases {
		parsed, err := parse(append(append([]string{}, issuer...), c.args...), func(key string) string { return c.env[key] })
		if err != nil {
			t.Fatal(err)
		}
		config, err := configure(parsed, withVerifier)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		case c.want == "" && config.Store == nil:
			t.Errorf("%s: no store", name)
		}
	}
}
