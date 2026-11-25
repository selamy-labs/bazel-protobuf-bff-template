package identity

import (
	"context"
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

type verifierFunc func(string) (string, error)

func (f verifierFunc) Verify(_ context.Context, token string) (string, error) { return f(token) }

func TestKindString(t *testing.T) {
	cases := []struct {
		name string
		kind Kind
		want string
	}{
		{"verified", KindVerified, "verified"},
		{"test identity", KindTestIdentity, "test-identity"},
		{"zero value", Kind(0), "unknown"},
		{"out of range", Kind(99), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			kind := tc.kind

			// Act
			got := kind.String()

			// Assert
			if got != tc.want {
				t.Fatalf("Kind(%d).String() = %q; want %q", kind, got, tc.want)
			}
		})
	}
}

func TestKindConstantsAreDistinctFromZero(t *testing.T) {
	cases := []struct {
		name  string
		check func() bool
	}{
		{"verified is nonzero", func() bool { return KindVerified != 0 }},
		{"test identity is nonzero", func() bool { return KindTestIdentity != 0 }},
		{"verified differs from test identity", func() bool { return KindVerified != KindTestIdentity }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			check := tc.check

			// Act
			ok := check()

			// Assert
			if !ok {
				t.Fatalf("kind invariant %q failed", tc.name)
			}
		})
	}
}

func TestBearerTokenValidHeaders(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   string
	}{
		{"canonical Bearer", "Bearer abc", "abc"},
		{"lowercase bearer", "bearer abc", "abc"},
		{"uppercase BEARER", "BEARER abc", "abc"},
		{"mixed case BeArEr", "BeArEr tok.1-2_3", "tok.1-2_3"},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/accepted", func(t *testing.T) {
			// Arrange
			header := tc.header

			// Act
			_, ok := BearerToken(header)

			// Assert
			if !ok {
				t.Fatalf("BearerToken(%q): ok=false; want true", header)
			}
		})

		t.Run(tc.name+"/token", func(t *testing.T) {
			// Arrange
			header := tc.header

			// Act
			token, _ := BearerToken(header)

			// Assert
			if token != tc.want {
				t.Fatalf("BearerToken(%q): token=%q; want %q", header, token, tc.want)
			}
		})
	}
}

func TestBearerTokenInvalidHeaders(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"empty header", ""},
		{"scheme without space", "Bearer"},
		{"empty token", "Bearer "},
		{"basic scheme", "Basic abc"},
		{"space inside token", "Bearer a b"},
		{"tab inside token", "Bearer a\tb"},
		{"bare token", "abc"},
	}

	for _, tc := range cases {
		t.Run(tc.name+"/rejected", func(t *testing.T) {
			// Arrange
			header := tc.header

			// Act
			_, ok := BearerToken(header)

			// Assert
			if ok {
				t.Fatalf("BearerToken(%q): ok=true; want false", header)
			}
		})

		t.Run(tc.name+"/empty token", func(t *testing.T) {
			// Arrange
			header := tc.header

			// Act
			token, _ := BearerToken(header)

			// Assert
			if token != "" {
				t.Fatalf("BearerToken(%q): token=%q; want empty", header, token)
			}
		})
	}
}

const tokenAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-._~+/="

type admissibleBearerHeader struct {
	header string
	token  string
}

func (admissibleBearerHeader) Generate(r *rand.Rand, _ int) reflect.Value {
	schemeChars := []byte("bearer")
	for i := range schemeChars {
		if r.Intn(2) == 1 {
			schemeChars[i] = byte(strings.ToUpper(string(schemeChars[i]))[0])
		}
	}
	n := 1 + r.Intn(64)
	var tok strings.Builder
	tok.Grow(n)
	for range n {
		tok.WriteByte(tokenAlphabet[r.Intn(len(tokenAlphabet))])
	}
	token := tok.String()
	return reflect.ValueOf(admissibleBearerHeader{
		header: string(schemeChars) + " " + token,
		token:  token,
	})
}

func TestBearerTokenAcceptsAdmissibleHeaderProperty(t *testing.T) {
	// Arrange
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleBearerHeader) bool {
		_, ok := BearerToken(sample.header)
		return ok
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("admissible bearer acceptance property failed: %v", err)
	}
}

func TestBearerTokenExtractsExactAdmissibleTokenProperty(t *testing.T) {
	// Arrange
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(sample admissibleBearerHeader) bool {
		got, _ := BearerToken(sample.header)
		return got == sample.token
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("admissible bearer token extraction property failed: %v", err)
	}
}

type whitespaceBearerHeader string

func (whitespaceBearerHeader) Generate(r *rand.Rand, _ int) reflect.Value {
	nLeft := 1 + r.Intn(16)
	nRight := 1 + r.Intn(16)
	var b strings.Builder
	b.WriteString("Bearer ")
	for range nLeft {
		b.WriteByte(tokenAlphabet[r.Intn(len(tokenAlphabet))])
	}
	if r.Intn(2) == 0 {
		b.WriteByte(' ')
	} else {
		b.WriteByte('\t')
	}
	for range nRight {
		b.WriteByte(tokenAlphabet[r.Intn(len(tokenAlphabet))])
	}
	return reflect.ValueOf(whitespaceBearerHeader(b.String()))
}

func TestBearerTokenRejectsWhitespaceInsideTokenProperty(t *testing.T) {
	// Arrange
	cfg := &quick.Config{Rand: rand.New(rand.NewSource(1))}
	property := func(raw whitespaceBearerHeader) bool {
		_, ok := BearerToken(string(raw))
		return !ok
	}

	// Act
	err := quick.Check(property, cfg)

	// Assert
	if err != nil {
		t.Fatalf("whitespace-in-token rejection property failed: %v", err)
	}
}

func TestNewBearerRefusesNoVerifier(t *testing.T) {
	// Arrange
	var verifier Verifier

	// Act
	_, err := NewBearer(verifier)

	// Assert
	if err == nil {
		t.Fatal("NewBearer(nil): got nil error; want error")
	}
}

func TestBearerAuthenticatorKindIsVerified(t *testing.T) {
	// Arrange
	authenticator, err := NewBearer(verifierFunc(func(string) (string, error) { return "s", nil }))
	if err != nil {
		t.Fatal(err)
	}

	// Act
	got := authenticator.Kind()

	// Assert
	if got != KindVerified {
		t.Fatalf("authenticator.Kind() = %v; want %v", got, KindVerified)
	}
}

func newTestAuthenticator(t *testing.T, outage error) Authenticator {
	t.Helper()
	authenticator, err := NewBearer(verifierFunc(func(token string) (string, error) {
		switch token {
		case "good":
			return "subject-1", nil
		case "empty":
			return "", nil
		case "outage":
			return "", outage
		default:
			return "", ErrUnauthenticated
		}
	}))
	if err != nil {
		t.Fatal(err)
	}
	return authenticator
}

func TestBearerAuthenticateValidTokenSucceeds(t *testing.T) {
	// Arrange
	authenticator := newTestAuthenticator(t, errors.New("verifier is down"))

	// Act
	_, err := authenticator.Authenticate(context.Background(), "Bearer good")

	// Assert
	if err != nil {
		t.Fatalf("valid token error: got %v; want nil", err)
	}
}

func TestBearerAuthenticateValidTokenReturnsVerifiedPrincipal(t *testing.T) {
	// Arrange
	authenticator := newTestAuthenticator(t, errors.New("verifier is down"))
	want := Principal{Subject: "subject-1", Kind: KindVerified}

	// Act
	got, _ := authenticator.Authenticate(context.Background(), "Bearer good")

	// Assert
	if got != want {
		t.Fatalf("valid token principal: got %+v; want %+v", got, want)
	}
}

func TestBearerAuthenticateRejectsUnauthenticatedCases(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{"missing header", ""},
		{"rejected token", "Bearer bad"},
		{"empty subject from verifier", "Bearer empty"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			authenticator := newTestAuthenticator(t, errors.New("verifier is down"))

			// Act
			_, err := authenticator.Authenticate(context.Background(), tc.header)

			// Assert
			if !errors.Is(err, ErrUnauthenticated) {
				t.Fatalf("%s (%q): got %v; want %v", tc.name, tc.header, err, ErrUnauthenticated)
			}
		})
	}
}

func TestBearerAuthenticatePreservesVerifierOutageError(t *testing.T) {
	// Arrange
	outage := errors.New("verifier is down")
	authenticator := newTestAuthenticator(t, outage)

	// Act
	_, err := authenticator.Authenticate(context.Background(), "Bearer outage")

	// Assert
	if !errors.Is(err, outage) {
		t.Fatalf("verifier outage: got %v; want %v", err, outage)
	}
}

func TestFromContextReturnsFalseWhenAbsent(t *testing.T) {
	// Arrange
	ctx := context.Background()

	// Act
	_, ok := FromContext(ctx)

	// Assert
	if ok {
		t.Fatal("FromContext(background): ok=true; want false")
	}
}

func TestFromContextReturnsTrueWhenPresent(t *testing.T) {
	// Arrange
	ctx := WithPrincipal(context.Background(), Principal{Subject: "ana", Kind: KindVerified})

	// Act
	_, ok := FromContext(ctx)

	// Assert
	if !ok {
		t.Fatal("FromContext(WithPrincipal): ok=false; want true")
	}
}

func TestFromContextReturnsStoredPrincipal(t *testing.T) {
	// Arrange
	want := Principal{Subject: "ana", Kind: KindVerified}
	ctx := WithPrincipal(context.Background(), want)

	// Act
	got, _ := FromContext(ctx)

	// Assert
	if got != want {
		t.Fatalf("FromContext(WithPrincipal): got %+v; want %+v", got, want)
	}
}
