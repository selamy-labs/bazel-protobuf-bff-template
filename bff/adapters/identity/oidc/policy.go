package oidc

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"errors"
	"strings"
	"time"

	"github.com/MicahParks/jwkset"
)

// Policy is required at composition. Zero leeway explicitly means no skew tolerance.
// Unknown keys may refresh at most once per UnknownRefresh, waiting at most UnknownWait.
type Policy struct {
	Leeway          time.Duration
	HTTPTimeout     time.Duration
	RefreshInterval time.Duration
	UnknownRefresh  time.Duration
	UnknownWait     time.Duration
	MaxBytes        int64
	MaxKeys         int
}

func (p Policy) Check() error {
	if p.Leeway < 0 || p.Leeway > time.Minute {
		return errors.New("verifier.leeway: outside zero to one minute")
	}
	if p.MaxBytes < 1 || p.MaxBytes > 1<<20 || p.MaxKeys < 1 || p.MaxKeys > 100 {
		return errors.New("verifier: required key/body bounds exceed safety ceilings")
	}
	return p.checkDurations()
}
func (p Policy) checkDurations() error {
	for _, d := range []time.Duration{p.HTTPTimeout, p.RefreshInterval, p.UnknownRefresh, p.UnknownWait} {
		if d <= 0 || d > 24*time.Hour {
			return errors.New("verifier: time bounds must be positive and at most 24 hours")
		}
	}
	return nil
}

func signingMetadata(m jwkset.JWKMarshal) bool {
	if m.KID == "" || (m.USE != "" && m.USE != jwkset.UseSig) {
		return false
	}
	if m.ALG != "" && !allowedMethod(string(m.ALG)) {
		return false
	}
	for _, operation := range m.KEYOPS {
		if string(operation) != "verify" {
			return false
		}
	}
	return true
}
func allowedMethod(algorithm string) bool {
	for _, method := range methods {
		if algorithm == method {
			return true
		}
	}
	return false
}
func eligible(key jwkset.JWK) bool {
	if !signingMetadata(key.Marshal()) {
		return false
	}
	switch k := key.Key().(type) {
	case *rsa.PublicKey:
		return rsaEligible(k, string(key.Marshal().ALG))
	case *ecdsa.PublicKey:
		return ecEligible(k, string(key.Marshal().ALG))
	case ed25519.PublicKey:
		return len(k) == ed25519.PublicKeySize && (key.Marshal().ALG == "" || key.Marshal().ALG == jwkset.AlgEdDSA)
	default:
		return false
	}
}

func rsaEligible(k *rsa.PublicKey, alg string) bool {
	return k.N.BitLen() >= 2048 && (alg == "" || strings.HasPrefix(alg, "RS") || strings.HasPrefix(alg, "PS"))
}
func ecEligible(k *ecdsa.PublicKey, alg string) bool {
	expected := map[int]string{256: "ES256", 384: "ES384", 521: "ES512"}[k.Curve.Params().BitSize]
	return expected != "" && (alg == "" || alg == expected)
}
