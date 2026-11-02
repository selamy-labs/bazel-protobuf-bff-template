// Package oidc verifies bearer tokens from an issuer that signs them as JSON Web Tokens and publishes its
// signing keys as a JSON Web Key Set, which is what an OpenID Connect provider does (spec 012 FR-015, T003,
// T007). It implements identity.Verifier for the production server: the issuer, the audience and the key set's
// URL are the composition's inputs, the keys are fetched once at start and kept refreshed, and a token is
// accepted only when its signature, issuer, audience and expiry all hold. Nothing here decides who may do what;
// it only says who a token is for.
package oidc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"

	"example.com/bfftemplate/bff/identity"
)

// Settings name the issuer whose tokens are accepted. Every field is required, and the composition declares them.
type Settings struct {
	// Issuer is the exact value a token's iss claim must carry.
	Issuer string
	// Audience is the value a token's aud claim must include.
	Audience string
	// JWKSURL is where the issuer publishes its signing keys.
	JWKSURL string
	Policy  Policy
}

// Check refuses an incomplete or implausible setting by name. The key set must be read over https, except from
// a loopback address, which a test issuer uses.
func (s Settings) Check() error {
	if s.Issuer == "" {
		return errors.New("verifier.issuer: required")
	}
	if s.Audience == "" {
		return errors.New("verifier.audience: required")
	}
	parsed, err := url.Parse(s.JWKSURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && loopback(parsed.Hostname()))) {
		return errors.New("verifier.jwks_url: HTTPS or literal loopback HTTP without userinfo or fragment required")
	}
	return s.Policy.Check()
}

func loopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// The signing algorithms accepted: asymmetric only, so a key set can never be misused as a shared secret.
var methods = []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"}

// Verifier accepts tokens signed by the issuer's current keys.
type Verifier struct {
	settings Settings
	keys     keyfunc.Keyfunc
	now      func() time.Time
}

// New checks the settings, fetches the issuer's keys once, and keeps them refreshed until ctx ends. It fails
// when the keys cannot be read, so a server never starts with no way to verify anything.
func New(ctx context.Context, settings Settings) (*Verifier, error) {
	if err := settings.Check(); err != nil {
		return nil, err
	}
	owned, cancel := context.WithCancel(ctx)
	keys, err := loadKeys(owned, settings)
	if err == nil {
		err = usableKeys(owned, keys)
	}
	if err != nil {
		cancel()
		return nil, fmt.Errorf("verifier: reading the key set: %w", err)
	}
	return &Verifier{settings: settings, keys: keys, now: time.Now}, nil
}

// Verify returns the token's subject, or identity.ErrUnauthenticated for a token it rejects. The reason it was
// rejected stays here: a caller learns only that the token is not accepted.
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	if strings.TrimSpace(token) == "" {
		return "", identity.ErrUnauthenticated
	}
	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(token, claims, v.signingKey(ctx),
		jwt.WithValidMethods(methods),
		jwt.WithIssuer(v.settings.Issuer),
		jwt.WithAudience(v.settings.Audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithLeeway(v.settings.Policy.Leeway),
		jwt.WithTimeFunc(v.now),
	)
	if err != nil || !parsed.Valid || claims.Subject == "" {
		return "", identity.ErrUnauthenticated
	}
	return claims.Subject, nil
}

func usableKeys(ctx context.Context, keys keyfunc.Keyfunc) error {
	all, err := keys.Storage().KeyReadAll(ctx)
	if err != nil {
		return err
	}
	for _, key := range all {
		if eligible(key) {
			return nil
		}
	}
	return errors.New("JWKS has no eligible asymmetric signing key")
}
func (v *Verifier) signingKey(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		kid, ok := token.Header["kid"].(string)
		if !ok || kid == "" {
			return nil, identity.ErrUnauthenticated
		}
		key, err := v.keys.Storage().KeyRead(ctx, kid)
		if err != nil || !eligible(key) {
			return nil, identity.ErrUnauthenticated
		}
		alg := string(key.Marshal().ALG)
		if alg != "" && alg != token.Method.Alg() {
			return nil, identity.ErrUnauthenticated
		}
		return key.Key(), nil
	}
}
