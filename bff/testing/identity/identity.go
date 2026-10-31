// Package identity is a development authenticator. It accepts any subject without proof, so it must
// never reach a production binary. The package is testonly in Bazel, and the production server
// refuses its principals.
package identity

import (
	"context"
	"regexp"
	"strings"

	"example.com/bfftemplate/bff/identity"
)

// Prefix labels a test credential, so it cannot be mistaken for a real token.
const Prefix = "test-identity."

var subjectPattern = regexp.MustCompile(`\A[a-z0-9-]{1,64}\z`)

// Authenticator accepts `Bearer test-identity.<subject>`.
type Authenticator struct{}

// Kind reports that this authenticator produces test principals.
func (Authenticator) Kind() identity.Kind { return identity.KindTestIdentity }

// Authenticate returns the principal named by the credential.
func (Authenticator) Authenticate(_ context.Context, authorization string) (identity.Principal, error) {
	token, ok := identity.BearerToken(authorization)
	if !ok {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	subject, found := strings.CutPrefix(token, Prefix)
	if !found || !subjectPattern.MatchString(subject) {
		return identity.Principal{}, identity.ErrUnauthenticated
	}
	return identity.Principal{Subject: subject, Kind: identity.KindTestIdentity}, nil
}

// Header returns the Authorization header value for a subject.
func Header(subject string) string { return "Bearer " + Prefix + subject }
