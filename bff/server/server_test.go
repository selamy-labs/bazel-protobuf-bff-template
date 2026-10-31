package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/bfftemplate/bff/adapters/memory"
	"example.com/bfftemplate/bff/identity"
	testidentity "example.com/bfftemplate/bff/testing/identity"
)

type verified struct{}

func (verified) Kind() identity.Kind { return identity.KindVerified }

func (verified) Authenticate(context.Context, string) (identity.Principal, error) {
	return identity.Principal{}, identity.ErrUnauthenticated
}

func valid() Config {
	return Config{
		Mode:          ModeProduction,
		Addr:          "127.0.0.1:0",
		AllowedOrigin: "https://app.example.com",
		Authenticator: verified{},
		Store:         memory.New(),
		Logger:        slog.New(slog.DiscardHandler),
	}
}

func TestCheck(t *testing.T) {
	dev := func(c *Config) { c.Mode, c.Authenticator = ModeDev, testidentity.Authenticator{} }
	cases := []struct {
		name   string
		change func(*Config)
		want   string
	}{
		{"production with a verifier", func(*Config) {}, ""},
		{"dev on loopback", dev, ""},
		{"dev on IPv6 loopback", func(c *Config) { dev(c); c.Addr = "[::1]:0" }, ""},
		{"production with test identity", func(c *Config) { c.Authenticator = testidentity.Authenticator{} }, "production mode refuses test-identity"},
		{"no authenticator", func(c *Config) { c.Authenticator = nil }, "no authenticator"},
		{"no store", func(c *Config) { c.Store = nil }, "no store"},
		{"dev with a verifier", func(c *Config) { c.Mode = ModeDev }, "dev mode expects test identity"},
		{"dev on every interface", func(c *Config) { dev(c); c.Addr = "0.0.0.0:0" }, "loopback"},
		{"dev on no host", func(c *Config) { dev(c); c.Addr = ":0" }, "loopback"},
		{"dev on a name", func(c *Config) { dev(c); c.Addr = "localhost:0" }, "loopback"},
		{"empty mode", func(c *Config) { c.Mode = "" }, "unknown mode"},
		{"wildcard origin", func(c *Config) { c.AllowedOrigin = "*" }, "exact"},
		{"empty origin", func(c *Config) { c.AllowedOrigin = "" }, "exact"},
		{"origin with a path", func(c *Config) { c.AllowedOrigin = "https://a.example/x" }, "exact"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config := valid()
			c.change(&config)
			err := config.Check()
			switch {
			case c.want == "" && err != nil:
				t.Fatalf("want no error, got %v", err)
			case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
				t.Fatalf("want %q, got %v", c.want, err)
			}
		})
	}
}

func TestRunRefusesBeforeListening(t *testing.T) {
	config := valid()
	config.Authenticator = testidentity.Authenticator{}
	var stdout bytes.Buffer
	config.Stdout = &stdout
	if err := Run(context.Background(), config); err == nil {
		t.Fatal("want a refusal")
	}
	if stdout.Len() != 0 {
		t.Fatalf("a refused server reported ready: %q", stdout.String())
	}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestRunPrintsReadyAndStops(t *testing.T) {
	config := valid()
	stdout := &lockedBuffer{}
	config.Stdout = stdout
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, config) }()

	var event readyEvent
	deadline := time.Now().Add(5 * time.Second)
	for stdout.String() == "" {
		if time.Now().After(deadline) {
			t.Fatal("no ready event")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := json.Unmarshal([]byte(stdout.String()), &event); err != nil {
		t.Fatal(err)
	}
	if event.Event != "ready" || event.Mode != ModeProduction || event.Identity != "verified" {
		t.Fatalf("unexpected event %+v", event)
	}
	connection, err := net.Dial("tcp", event.Addr)
	if err != nil {
		t.Fatalf("not listening at the reported address: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("did not stop")
	}
}
