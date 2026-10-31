package main

import "example.com/bfftemplate/bff/identity"

// newVerifier builds the token verifier. It is nil because this repository ships no identity
// provider, so the server refuses to start. Assign a constructor for your provider here.
var newVerifier func() (identity.Verifier, error)
