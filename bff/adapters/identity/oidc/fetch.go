package oidc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"golang.org/x/time/rate"
)

// boundedTransport validates a complete bounded document before the library replaces its cache.
// Rejected responses cannot partially install keys. All refreshes use the same policy.
type boundedTransport struct {
	next   http.RoundTripper
	policy Policy
}

func (t boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	response, err := t.next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("JWKS HTTP status refused")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, t.policy.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > t.policy.MaxBytes {
		return nil, errors.New("JWKS byte limit exceeded")
	}
	if err = checkDocument(data, t.policy.MaxKeys); err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	response.ContentLength = int64(len(data))
	return response, nil
}
func checkDocument(data []byte, maximum int) error {
	var set jwkset.JWKSMarshal
	if err := json.Unmarshal(data, &set); err != nil {
		return errors.New("JWKS must be a complete JSON document")
	}
	if len(set.Keys) == 0 || len(set.Keys) > maximum {
		return errors.New("JWKS key count outside bound")
	}
	seen := make(map[string]bool)
	for _, key := range set.Keys {
		if key.KID == "" || seen[key.KID] {
			return errors.New("JWKS key identifiers must be present and unique")
		}
		seen[key.KID] = true
	}
	return nil
}
func loadKeys(ctx context.Context, s Settings) (keyfunc.Keyfunc, error) {
	client := &http.Client{Timeout: s.Policy.HTTPTimeout, Transport: boundedTransport{next: http.DefaultTransport, policy: s.Policy}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	store, err := jwkset.NewStorageFromHTTP(s.JWKSURL, jwkset.HTTPClientStorageOptions{
		Ctx: ctx, Client: client, HTTPTimeout: s.Policy.HTTPTimeout, HTTPMethod: http.MethodGet, HTTPExpectedStatus: http.StatusOK,
		RefreshInterval: s.Policy.RefreshInterval, NoErrorReturnFirstHTTPReq: false, RequireSupportedKeys: true, Storage: jwkset.NewMemoryStorage(),
		RefreshErrorHandler: func(ctx context.Context, _ error) { slog.ErrorContext(ctx, "verifier key refresh failed") },
	})
	if err != nil {
		return nil, err
	}
	pool, err := jwkset.NewHTTPClient(jwkset.HTTPClientOptions{Given: jwkset.NewMemoryStorage(), HTTPURLs: map[string]jwkset.Storage{s.JWKSURL: store}, RateLimitWaitMax: s.Policy.UnknownWait, RefreshUnknownKID: rate.NewLimiter(rate.Every(s.Policy.UnknownRefresh), 1)})
	if err != nil {
		return nil, err
	}
	return keyfunc.New(keyfunc.Options{Ctx: ctx, Storage: pool, UseWhitelist: []jwkset.USE{jwkset.UseSig, ""}})
}
