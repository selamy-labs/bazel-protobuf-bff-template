package oidc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"

	"example.com/bfftemplate/bff/identity"
)

// A synthetic issuer: keys generated here, published as a key set by a local server, tokens signed on demand.
type issuer struct {
	t        *testing.T
	rsaKey   *rsa.PrivateKey
	ecKey    *ecdsa.PrivateKey
	other    *rsa.PrivateKey
	server   *httptest.Server
	settings Settings
}

func newIssuer(t *testing.T) *issuer {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	published := jwkset.JWKSMarshal{}
	for kid, key := range map[string]any{"rsa": rsaKey.Public(), "ec": ecKey.Public()} {
		jwk, err := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{Metadata: jwkset.JWKMetadataOptions{KID: kid, USE: jwkset.UseSig}})
		if err != nil {
			t.Fatal(err)
		}
		published.Keys = append(published.Keys, jwk.Marshal())
	}
	body, err := json.Marshal(published)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	return &issuer{t: t, rsaKey: rsaKey, ecKey: ecKey, other: other, server: server, settings: Settings{
		Issuer:   "https://issuer.example.invalid/",
		Audience: "bff-hosted-example",
		JWKSURL:  server.URL + "/.well-known/jwks.json",
	}}
}

type tokenOptions struct {
	issuer, audience, subject, kid string
	expires                        time.Time
	issued                         time.Time
	method                         jwt.SigningMethod
	key                            any
}

func (i *issuer) token(o tokenOptions) string {
	i.t.Helper()
	claims := jwt.MapClaims{"iss": o.issuer, "aud": o.audience, "sub": o.subject, "exp": o.expires.Unix(), "iat": o.issued.Unix()}
	if o.subject == "" {
		delete(claims, "sub")
	}
	if o.expires.IsZero() {
		delete(claims, "exp")
	}
	token := jwt.NewWithClaims(o.method, claims)
	token.Header["kid"] = o.kid
	signed, err := token.SignedString(o.key)
	if err != nil {
		i.t.Fatal(err)
	}
	return signed
}

func (i *issuer) good() tokenOptions {
	now := time.Now()
	return tokenOptions{issuer: i.settings.Issuer, audience: i.settings.Audience, subject: "ana", kid: "rsa", expires: now.Add(time.Hour), issued: now, method: jwt.SigningMethodRS256, key: i.rsaKey}
}

func TestAValidTokenYieldsItsSubject(t *testing.T) {
	issuer := newIssuer(t)
	verifier, err := New(context.Background(), issuer.settings)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := verifier.Verify(context.Background(), issuer.token(issuer.good()))
	if err != nil || subject != "ana" {
		t.Fatalf("subject %q, err %v", subject, err)
	}
	ec := issuer.good()
	ec.kid, ec.method, ec.key, ec.subject = "ec", jwt.SigningMethodES256, issuer.ecKey, "bob"
	subject, err = verifier.Verify(context.Background(), issuer.token(ec))
	if err != nil || subject != "bob" {
		t.Fatalf("an ES256 token: subject %q, err %v", subject, err)
	}
}

// Each refusal is ErrUnauthenticated and nothing more: the caller learns no reason.
func TestEveryBadTokenIsUnauthenticated(t *testing.T) {
	issuer := newIssuer(t)
	verifier, err := New(context.Background(), issuer.settings)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	cases := map[string]func(o *tokenOptions){
		"wrong issuer":          func(o *tokenOptions) { o.issuer = "https://other.example.invalid/" },
		"wrong audience":        func(o *tokenOptions) { o.audience = "someone-else" },
		"expired":               func(o *tokenOptions) { o.expires = now.Add(-time.Hour) },
		"no expiry":             func(o *tokenOptions) { o.expires = time.Time{} },
		"issued in the future":  func(o *tokenOptions) { o.issued = now.Add(time.Hour) },
		"bad signature":         func(o *tokenOptions) { o.key = issuer.other },
		"unknown key":           func(o *tokenOptions) { o.kid = "retired" },
		"no subject":            func(o *tokenOptions) { o.subject = "" },
		"symmetric algorithm":   func(o *tokenOptions) { o.method, o.key = jwt.SigningMethodHS256, []byte("shared") },
		"algorithm none":        func(o *tokenOptions) { o.method, o.key = jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType },
		"key of the other kind": func(o *tokenOptions) { o.kid, o.key = "ec", issuer.rsaKey },
	}
	for name, change := range cases {
		options := issuer.good()
		change(&options)
		subject, err := verifier.Verify(context.Background(), issuer.token(options))
		if !errors.Is(err, identity.ErrUnauthenticated) || subject != "" {
			t.Errorf("%s: subject %q, err %v; want unauthenticated", name, subject, err)
		}
		if err != nil && err.Error() != identity.ErrUnauthenticated.Error() {
			t.Errorf("%s: the refusal carries a reason: %v", name, err)
		}
	}
	for name, raw := range map[string]string{"empty": "", "blank": "   ", "not a token": "abc.def", "garbage": strings.Repeat("x", 100)} {
		if _, err := verifier.Verify(context.Background(), raw); !errors.Is(err, identity.ErrUnauthenticated) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSettingsAreCheckedByName(t *testing.T) {
	issuer := newIssuer(t)
	cases := map[string]struct {
		change func(*Settings)
		want   string
	}{
		"no issuer":        {func(s *Settings) { s.Issuer = "" }, "verifier.issuer: required"},
		"no audience":      {func(s *Settings) { s.Audience = "" }, "verifier.audience: required"},
		"plain http keys":  {func(s *Settings) { s.JWKSURL = "http://issuer.example.invalid/jwks" }, "verifier.jwks_url"},
		"no host":          {func(s *Settings) { s.JWKSURL = "https:///jwks" }, "verifier.jwks_url"},
		"not a url scheme": {func(s *Settings) { s.JWKSURL = "ftp://issuer.example.invalid/jwks" }, "verifier.jwks_url"},
	}
	for name, c := range cases {
		settings := issuer.settings
		c.change(&settings)
		_, err := New(context.Background(), settings)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", name, err, c.want)
		}
	}
	if err := issuer.settings.Check(); err != nil {
		t.Errorf("the issuer's own settings: %v", err)
	}
}

// A server never starts with keys it could not read.
func TestUnreadableKeysRefuseToStart(t *testing.T) {
	issuer := newIssuer(t)
	issuer.server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := New(ctx, issuer.settings); err == nil || !strings.Contains(err.Error(), "reading the key set") {
		t.Fatalf("got %v", err)
	}
	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("not a key set")) }))
	defer broken.Close()
	settings := issuer.settings
	settings.JWKSURL = broken.URL
	if _, err := New(ctx, settings); err == nil {
		t.Fatal("a key set that does not parse was accepted")
	}
}

// Clock skew within the leeway is tolerated, beyond it is not.
func TestLeeway(t *testing.T) {
	issuer := newIssuer(t)
	verifier, err := New(context.Background(), issuer.settings)
	if err != nil {
		t.Fatal(err)
	}
	within := issuer.good()
	within.expires = time.Now().Add(-leeway / 2)
	if _, err := verifier.Verify(context.Background(), issuer.token(within)); err != nil {
		t.Errorf("expired within the leeway: %v", err)
	}
	beyond := issuer.good()
	beyond.expires = time.Now().Add(-2 * leeway)
	if _, err := verifier.Verify(context.Background(), issuer.token(beyond)); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Errorf("expired beyond the leeway: %v", err)
	}
}
