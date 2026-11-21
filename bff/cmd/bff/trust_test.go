// Production composition trust witnesses (spec009 T019a). System roots are absent in this target.
package main

import (
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/x509roots/fallback/bundle"
)

func TestEmbeddedPublicRootWithoutSystemBundle(t *testing.T) {
	// The bundle subpackage only supplies data; it does not register fallback roots.
	// Only the production composition's import can make this default-root verification succeed.
	for root := range bundle.Roots() {
		if root.Constraint != nil {
			continue
		}
		cert, err := x509.ParseCertificate(root.Certificate)
		if err != nil {
			t.Fatal(err)
		}
		chains, err := cert.Verify(x509.VerifyOptions{CurrentTime: cert.NotBefore.Add(24 * time.Hour), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
		if err != nil {
			t.Fatalf("embedded public trust anchor is unavailable without system roots: %v", err)
		}
		if len(chains) != 1 || len(chains[0]) != 1 || !chains[0][0].Equal(cert) {
			t.Fatal("unexpected trust-anchor chain")
		}
		return
	}
	t.Fatal("the pinned bundle contains no unconstrained public root")
}

func TestEmbeddedRootsRefuseUntrustedHTTPS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "must remain untrusted") }))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Get(server.URL)
	if response != nil {
		_ = response.Body.Close()
	}
	var unknown x509.UnknownAuthorityError
	if !errors.As(err, &unknown) {
		t.Fatalf("want unknown-authority refusal, got %v", err)
	}
}
