package signer

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/rankegraph/ranke-db/adapters/signer/azure/azuretest"
	"github.com/rankegraph/ranke-db/adapters/signer/inmemory/inmemorytest"
	"github.com/rankegraph/ranke-db/adapters/signer/openbao/openbaotest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// backend is one signer backend under conformance. setup is its hook: it builds
// the backend's config — spinning up a real counterpart when it needs one (and
// t.Skip-ing when that is unavailable) — and returns a teardown. The driver calls
// every backend's setup the same way; each backend's setup lives beside the
// backend, out of this port test and out of the production binary.
type backend struct {
	name  string
	setup func(t *testing.T) (scope.Section, func())
}

var backends = []backend{
	{name: "inmemory", setup: inmemorytest.Setup},
	{name: "openbao", setup: openbaotest.Setup},
	{name: "azure", setup: azuretest.Setup},
	{name: "azure-secret", setup: azuretest.SetupSecret},
}

// TestConformance runs every backend through its setup, the test-view
// constructor, and the shared suite.
func TestConformance(t *testing.T) {
	for _, b := range backends {
		t.Run(b.name, func(t *testing.T) {
			cfg, teardown := b.setup(t)
			defer teardown()
			s, err := newTestSigner(context.Background(), cfg)
			if err != nil {
				t.Fatalf("newTestSigner: %v", err)
			}
			conform(t, s)
		})
	}
}

// conform is the shared signer contract, in whichever scheme the backend signs
// under: the prepared key is the one the signer signs with, a valid signature
// verifies, and a corrupted one does not.
func conform(t *testing.T, s testSigner) {
	t.Helper()
	ctx := context.Background()

	pub, err := s.PrepareKey(ctx, "conformance")
	if err != nil {
		t.Fatalf("PrepareKey: %v", err)
	}
	verify := verifier(t, pub)
	t.Logf("▸ prepared %s", Identity(ctx, s))

	got, err := s.Public(ctx)
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if !got.(interface{ Equal(crypto.PublicKey) bool }).Equal(pub) {
		t.Fatal("Public() does not match the prepared key")
	}
	t.Log("▸ Public() reports the prepared key")

	// A SHA-256 digest is what the port carries, and the only length ES256 signs.
	digest := sha256.Sum256([]byte("ranke-db signer conformance"))
	sig, err := s.Sign(ctx, digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	t.Logf("▸ signed a %d-byte digest → %d-byte signature", len(digest), len(sig))
	if !verify(digest[:], sig) {
		t.Fatal("valid signature did not verify")
	}
	t.Log("▸ signature verifies against the public key")

	sig[0] ^= 0xff
	if verify(digest[:], sig) {
		t.Fatal("corrupted signature verified")
	}
	t.Log("▸ corrupted signature is rejected")
}

// verifier is the check for the scheme pub is a key of: raw Ed25519, or ECDSA over
// P-256 whose signature is the ASN.1 DER a crypto.Signer answers with.
func verifier(t *testing.T, pub crypto.PublicKey) func(digest, sig []byte) bool {
	t.Helper()
	switch key := pub.(type) {
	case ed25519.PublicKey:
		return func(digest, sig []byte) bool { return ed25519.Verify(key, digest, sig) }
	case *ecdsa.PublicKey:
		return func(digest, sig []byte) bool { return ecdsa.VerifyASN1(key, digest, sig) }
	default:
		t.Fatalf("PrepareKey returned a %T, which `V-SIGN` names no scheme for", pub)
		return nil
	}
}

// TestNewRejects covers the configs New must reject: an empty or unknown backend
// type, and inmemory key material that is missing, non-PEM, or not ed25519.
func TestNewRejects(t *testing.T) {
	ctx := context.Background()
	bad := map[string]scope.Section{
		"empty type":            scope.Literal(map[string]string{}),
		"unknown type":          scope.Literal(map[string]string{"type": "nope"}),
		"inmemory missing key":  scope.Literal(map[string]string{"type": "inmemory"}),
		"inmemory non-PEM key":  scope.Literal(map[string]string{"type": "inmemory", "key": "not-a-pem"}),
		"inmemory non-ed25519":  scope.Literal(map[string]string{"type": "inmemory", "key": inmemorytest.ECDSAPEM(t)}),
		"azure without a vault": scope.Literal(map[string]string{"type": "azure"}),
		"azure two identities": scope.Literal(map[string]string{
			"type": "azure", "url": "https://v.vault.azure.net/", "key": "a-key", "secret": "a-secret",
		}),
	}
	for name, cfg := range bad {
		if _, err := New(ctx, cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}
