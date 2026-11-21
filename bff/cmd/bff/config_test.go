package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// The production composition serves its own built assets, not the separate development web server.
func productionWebServer(t *testing.T, root, clientID string) *httptest.Server {
	t.Helper()
	parsed, err := parse([]string{"--allowed-origin=https://app.example.com", "--verifier-issuer=https://issuer.example/", "--verifier-audience=bff", "--verifier-jwks-url=https://issuer.example/jwks"}, func(name string) string {
		return map[string]string{"BFF_WEB_ROOT": root, "BFF_SIGNIN_CLIENT_ID": clientID}[name]
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := configure(parsed, withVerifier)
	if err != nil {
		t.Fatal(err)
	}
	config.Logger = slog.New(slog.DiscardHandler)
	site := httptest.NewServer(server.NewHandler(config))
	t.Cleanup(site.Close)
	return site
}

func webFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range map[string]string{"index.html": "<main>Built notes</main>", "main.js": "console.log('built notes');"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "private-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestProductionServesItsBuiltAssets(t *testing.T) {
	site := productionWebServer(t, webFixture(t), "")
	for route, want := range map[string]string{"/": "<main>Built notes</main>", "/main.js": "console.log('built notes');"} {
		t.Run(route, func(t *testing.T) {
			response, err := site.Client().Get(site.URL + route)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || string(body) != want {
				t.Fatalf("built asset %s: status %d, body %q", route, response.StatusCode, body)
			}
		})
	}
}

func TestProductionWebConfigUsesTheSameOrigin(t *testing.T) {
	for clientID, want := range map[string]string{"": "{\"bffUrl\":\"\"}\n", "browser-client": "{\"bffUrl\":\"\",\"signIn\":{\"issuer\":\"https://issuer.example/\",\"clientId\":\"browser-client\"}}\n"} {
		t.Run(clientID, func(t *testing.T) {
			site := productionWebServer(t, webFixture(t), clientID)
			response, err := site.Client().Get(site.URL + "/config.json")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK || string(body) != want {
				t.Fatalf("same-origin config: status %d, body %q", response.StatusCode, body)
			}
		})
	}
}

func TestProductionWebDoesNotListDirectoriesOrInventAssets(t *testing.T) {
	site := productionWebServer(t, webFixture(t), "")
	for _, route := range []string{"/missing.js", "/private-directory/"} {
		t.Run(route, func(t *testing.T) {
			response, err := site.Client().Get(site.URL + route)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("unexpected asset %s: status %d", route, response.StatusCode)
			}
		})
	}
}

func TestProductionAssetsDoNotReplaceRPCAuthentication(t *testing.T) {
	site := productionWebServer(t, webFixture(t), "")
	response, err := site.Client().Post(site.URL+"/template.notes.v1.NoteService/CreateNote", "application/json", strings.NewReader(`{"note":{"title":"unauthenticated"}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("RPC without bearer: status %d", response.StatusCode)
	}
}

func TestProductionAssetsConfineSymlinks(t *testing.T) {
	root := webFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("not a browser asset"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	site := productionWebServer(t, root, "")
	response, err := site.Client().Get(site.URL + "/escape.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("outside-root symlink: status %d", response.StatusCode)
	}
}

func TestProductionAssetsKeepHTTPMethodsAndTypes(t *testing.T) {
	site := productionWebServer(t, webFixture(t), "")
	for _, c := range []struct {
		method, route, contentType string
		status                     int
	}{
		{http.MethodHead, "/", "text/html", http.StatusOK},
		{http.MethodHead, "/main.js", "text/javascript", http.StatusOK},
		{http.MethodHead, "/config.json", "application/json", http.StatusOK},
		{http.MethodPost, "/", "text/plain", http.StatusNotFound},
	} {
		t.Run(c.method+c.route, func(t *testing.T) {
			request, err := http.NewRequestWithContext(t.Context(), c.method, site.URL+c.route, nil)
			if err != nil {
				t.Fatal(err)
			}
			response, err := site.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != c.status || !strings.HasPrefix(response.Header.Get("Content-Type"), c.contentType) {
				t.Fatalf("%s %s: status %d, type %q", c.method, c.route, response.StatusCode, response.Header.Get("Content-Type"))
			}
		})
	}
}

func TestProductionWebRootAndSignInClientPrecedence(t *testing.T) {
	for _, c := range []struct {
		args           []string
		env            map[string]string
		root, clientID string
	}{
		{nil, nil, "/web", ""},
		{nil, map[string]string{"BFF_WEB_ROOT": "/built", "BFF_SIGNIN_CLIENT_ID": "browser"}, "/built", "browser"},
		{[]string{"--web-root=/selected", "--signin-client-id=selected"}, map[string]string{"BFF_WEB_ROOT": "/built", "BFF_SIGNIN_CLIENT_ID": "browser"}, "/selected", "selected"},
	} {
		parsed, err := parse(c.args, func(name string) string { return c.env[name] })
		if err != nil || parsed.webRoot != c.root || parsed.signInClientID != c.clientID {
			t.Fatalf("web configuration: root %q, client %q, error %v", parsed.webRoot, parsed.signInClientID, err)
		}
	}
}
