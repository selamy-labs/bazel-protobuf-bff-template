// Package identity is the authentication boundary. Identity comes only from the Authorization
// header, which a browser never sends on its own, so a cookie can never authenticate a call.
//
// This repository ships no identity provider. An adopter implements Verifier, and nothing here
// returns success without one.
package identity

import (
	"context"
	"errors"
	"strings"
)

// Kind says how a principal was established.
type Kind int

const (
	// KindVerified means a configured Verifier accepted a credential.
	KindVerified Kind = iota + 1
	// KindTestIdentity means a development credential was accepted. Production refuses it.
	KindTestIdentity
)

func (k Kind) String() string {
	switch k {
	case KindVerified:
		return "verified"
	case KindTestIdentity:
		return "test-identity"
	default:
		return "unknown"
	}
}

// ErrUnauthenticated means the call carries no acceptable credential.
var ErrUnauthenticated = errors.New("unauthenticated")

// Principal is the authenticated caller.
type Principal struct {
	Subject string
	Kind    Kind
}

// Authenticator turns an Authorization header value into a Principal.
type Authenticator interface {
	Authenticate(ctx context.Context, authorization string) (Principal, error)
	// Kind is the kind of principal this authenticator produces.
	Kind() Kind
}

// Verifier checks a bearer token and returns its opaque subject. It returns ErrUnauthenticated for
// a token it rejects. Any other error means it could not decide, and the call fails.
type Verifier interface {
	Verify(ctx context.Context, token string) (subject string, err error)
}

// BearerToken extracts the token from an Authorization header value.
func BearerToken(authorization string) (string, bool) {
	scheme, token, found := strings.Cut(authorization, " ")
	if !found || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t") {
		return "", false
	}
	return token, true
}

type bearer struct{ verifier Verifier }

// NewBearer returns an Authenticator that accepts bearer tokens the verifier accepts.
func NewBearer(verifier Verifier) (Authenticator, error) {
	if verifier == nil {
		return nil, errors.New("no verifier is configured")
	}
	return bearer{verifier}, nil
}

func (b bearer) Kind() Kind { return KindVerified }

func (b bearer) Authenticate(ctx context.Context, authorization string) (Principal, error) {
	token, ok := BearerToken(authorization)
	if !ok {
		return Principal{}, ErrUnauthenticated
	}
	subject, err := b.verifier.Verify(ctx, token)
	if err != nil {
		return Principal{}, err
	}
	if subject == "" {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{Subject: subject, Kind: KindVerified}, nil
}

type contextKey struct{}

// WithPrincipal returns a context carrying the principal.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

// FromContext returns the context's principal.
func FromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}
