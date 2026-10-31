package identity

import (
	"context"
	"errors"
	"testing"
)

type verifierFunc func(string) (string, error)

func (f verifierFunc) Verify(_ context.Context, token string) (string, error) { return f(token) }

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		token  string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"BEARER abc", "abc", true},
		{"", "", false},
		{"Bearer", "", false},
		{"Bearer ", "", false},
		{"Basic abc", "", false},
		{"Bearer a b", "", false},
		{"abc", "", false},
	}
	for _, c := range cases {
		token, ok := BearerToken(c.header)
		if token != c.token || ok != c.ok {
			t.Errorf("%q: got %q, %v", c.header, token, ok)
		}
	}
}

func TestNewBearerRefusesNoVerifier(t *testing.T) {
	if _, err := NewBearer(nil); err == nil {
		t.Fatal("want an error without a verifier")
	}
}

func TestBearerAuthenticator(t *testing.T) {
	outage := errors.New("verifier is down")
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
	ctx := context.Background()
	principal, err := authenticator.Authenticate(ctx, "Bearer good")
	if err != nil || principal != (Principal{"subject-1", KindVerified}) {
		t.Fatalf("got %+v, %v", principal, err)
	}
	for _, header := range []string{"", "Bearer bad", "Bearer empty"} {
		if _, err := authenticator.Authenticate(ctx, header); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%q: want unauthenticated, got %v", header, err)
		}
	}
	if _, err := authenticator.Authenticate(ctx, "Bearer outage"); !errors.Is(err, outage) {
		t.Errorf("a verifier failure must not read as a rejected credential: %v", err)
	}
}
