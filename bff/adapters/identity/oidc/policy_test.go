package oidc

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/bfftemplate/bff/identity"
	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
)

func serveKeys(t *testing.T, i *issuer, entries []jwkset.JWKMarshal) Settings {
	t.Helper()
	data, err := json.Marshal(jwkset.JWKSMarshal{Keys: entries})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	t.Cleanup(server.Close)
	s := i.settings
	s.JWKSURL = server.URL
	return s
}
func publicKey(t *testing.T, key any, kid string) jwkset.JWKMarshal {
	t.Helper()
	j, err := jwkset.NewJWKFromKey(key, jwkset.JWKOptions{Metadata: jwkset.JWKMetadataOptions{KID: kid, USE: jwkset.UseSig}})
	if err != nil {
		t.Fatal(err)
	}
	return j.Marshal()
}
func TestKeyPurposeAndMissingKidAreRefused(t *testing.T) {
	i := newIssuer(t)
	good := publicKey(t, i.ecKey.Public(), "ec")
	rsa := publicKey(t, i.rsaKey.Public(), "rsa")
	for _, kind := range []string{"enc", "wrong-alg", "key-ops"} {
		t.Run(kind, func(t *testing.T) {
			bad := rsa
			switch kind {
			case "enc":
				bad.USE = jwkset.UseEnc
			case "wrong-alg":
				bad.ALG = jwkset.AlgRS512
			case "key-ops":
				bad.KEYOPS = []jwkset.KEYOPS{"encrypt"}
				bad.USE = ""
			}
			verifier, err := New(t.Context(), serveKeys(t, i, []jwkset.JWKMarshal{good, bad}))
			if err != nil {
				if kind == "key-ops" {
					return
				}
				t.Fatal(err)
			}
			if _, err = verifier.Verify(t.Context(), i.token(i.good())); !errors.Is(err, identity.ErrUnauthenticated) {
				t.Fatalf("accepted %s: %v", kind, err)
			}
		})
	}
	verifier, err := New(t.Context(), i.settings)
	if err != nil {
		t.Fatal(err)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": i.settings.Issuer, "aud": i.settings.Audience, "sub": "ana", "exp": time.Now().Add(time.Hour).Unix()})
	text, err := token.SignedString(i.rsaKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = verifier.Verify(t.Context(), text); !errors.Is(err, identity.ErrUnauthenticated) {
		t.Fatalf("missing kid accepted: %v", err)
	}
}
func TestStartupRequiresEligibleSigningKeys(t *testing.T) {
	i := newIssuer(t)
	enc := publicKey(t, i.rsaKey.Public(), "enc")
	enc.USE = jwkset.UseEnc
	symmetric := jwkset.JWKMarshal{KTY: jwkset.KtyOct, KID: "sym", USE: jwkset.UseSig, ALG: jwkset.AlgHS256, K: "a2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2tra2s"}
	for _, key := range []jwkset.JWKMarshal{enc, symmetric} {
		if _, err := New(t.Context(), serveKeys(t, i, []jwkset.JWKMarshal{key})); err == nil {
			t.Fatal("accepted ineligible-only set")
		}
	}
}
func TestFetchRefusesRedirectAndBodyBounds(t *testing.T) {
	i := newIssuer(t)
	targetCalls := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls <- struct{}{} }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer redirect.Close()
	s := i.settings
	s.JWKSURL = redirect.URL
	if _, err := New(t.Context(), s); err == nil {
		t.Fatal("redirect accepted")
	}
	select {
	case <-targetCalls:
		t.Fatal("redirect target contacted")
	default:
	}
	s = i.settings
	s.Policy.MaxBytes = 8
	if _, err := New(t.Context(), s); err == nil {
		t.Fatal("oversized document accepted")
	}
	s = i.settings
	s.Policy.MaxKeys = 1
	if _, err := New(t.Context(), s); err == nil {
		t.Fatal("key count bound ignored")
	}
	s = i.settings
	s.JWKSURL = "https://name:password@example.invalid/keys"
	if s.Check() == nil {
		t.Fatal("userinfo accepted")
	}
	for _, raw := range []string{`{"keys":[]} trailing`, `{"keys":[{"kid":"a"},{"kid":"a"}]}`, `{"keys":[{}]}`} {
		if checkDocument([]byte(raw), 4) == nil {
			t.Fatal("invalid document accepted")
		}
	}
}
func TestStartupCancellation(t *testing.T) {
	i := newIssuer(t)
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-release }))
	defer server.Close()
	defer close(release)
	s := i.settings
	s.JWKSURL = server.URL
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := New(ctx, s); done <- err }()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("startup ignored cancellation")
	}
}

func TestInvalidURLDoesNotEchoCredentials(t *testing.T) {
	s := Settings{Issuer: "issuer", Audience: "audience", JWKSURL: "https://user:PRIVATE_URL_PASSWORD@example.invalid/jwks", Policy: testPolicy()}
	err := s.Check()
	if err == nil || strings.Contains(err.Error(), "PRIVATE_URL_PASSWORD") {
		t.Fatalf("unsafe error: %v", err)
	}
}
