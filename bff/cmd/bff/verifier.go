package main

import (
	"context"

	"example.com/bfftemplate/bff/adapters/identity/oidc"
	"example.com/bfftemplate/bff/identity"
)

// newVerifier builds the token verifier for the issuer the composition names (spec 012 FR-015): tokens signed
// with the keys that issuer publishes. The key set is read once here and refreshed for the life of the process.
// This repository ships no identity provider; without an issuer named, configure refuses before reaching here.
var newVerifier = func(settings oidc.Settings) (identity.Verifier, error) {
	return oidc.New(context.Background(), settings)
}
