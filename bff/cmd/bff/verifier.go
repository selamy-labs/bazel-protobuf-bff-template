package main

import (
	"context"
	"example.com/bfftemplate/bff/adapters/identity/oidc"
	"example.com/bfftemplate/bff/identity"
	"time"
)

// Bounds are declared once at composition, never supplied by adapter fallback.
func verifierPolicy() oidc.Policy {
	return oidc.Policy{Leeway: 30 * time.Second, HTTPTimeout: 5 * time.Second, RefreshInterval: time.Hour, UnknownRefresh: 5 * time.Minute, UnknownWait: 100 * time.Millisecond, MaxBytes: 64 << 10, MaxKeys: 32}
}
func newVerifier(ctx context.Context, settings oidc.Settings) (identity.Verifier, error) {
	return oidc.New(ctx, settings)
}
