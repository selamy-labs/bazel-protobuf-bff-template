package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/identity"
	"example.com/bfftemplate/bff/server"
)

type verifier struct{}

func (verifier) Verify(context.Context, string) (string, error) {
	return "", identity.ErrUnauthenticated
}

func withVerifier() (identity.Verifier, error) { return verifier{}, nil }

func noEnvironment(string) string { return "" }

func TestShippedBinaryHasNoVerifier(t *testing.T) {
	if newVerifier != nil {
		t.Fatal("this repository ships no verifier")
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
		verifier func() (identity.Verifier, error)
		want     string
	}{
		{name: "no verifier", want: "no verifier is configured"},
		{name: "a failing verifier", verifier: func() (identity.Verifier, error) { return nil, errors.New("no key") }, want: "no key"},
		{name: "a nil verifier", verifier: func() (identity.Verifier, error) { return nil, nil }, want: "no verifier is configured"},
		{name: "test identity by flag", args: []string{"--identity=test-identity"}, verifier: withVerifier, want: "refuses identity"},
		{name: "test identity by environment", env: map[string]string{"BFF_IDENTITY": "test-identity"}, verifier: withVerifier, want: "refuses identity"},
		{name: "dev mode by flag", args: []string{"--mode=dev"}, verifier: withVerifier, want: "only in production"},
		{name: "dev mode by environment", env: map[string]string{"BFF_MODE": "dev"}, verifier: withVerifier, want: "only in production"},
		{name: "with a verifier", verifier: withVerifier},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			parsed, err := parse(c.args, func(key string) string { return c.env[key] })
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
			if config.Mode != server.ModeProduction || config.Authenticator.Kind() != identity.KindVerified {
				t.Fatalf("unexpected config %+v", config)
			}
		})
	}
}

func TestConfiguredProductionPassesTheServerCheck(t *testing.T) {
	parsed, err := parse([]string{"--allowed-origin=https://app.example.com"}, noEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	config, err := configure(parsed, withVerifier)
	if err != nil {
		t.Fatal(err)
	}
	config.Store = memory.New()
	if err := config.Check(); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownFlagIsAnError(t *testing.T) {
	if _, err := parse([]string{"--no-such-flag"}, noEnvironment); err == nil {
		t.Fatal("want an error")
	}
}
