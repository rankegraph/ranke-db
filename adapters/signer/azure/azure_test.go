package azure

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"testing"

	"github.com/rankegraph/ranke-db/adapters/vault/azure/azuretest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// TestTheVaultSigns covers the launch path for the vault-signing identity: a signer
// given only a key name has the vault sign under ES256, and the signature verifies
// against the public half the vault publishes. One signer provisions the key, a
// second — configured the way a deployment configures one — signs with it.
func TestTheVaultSigns(t *testing.T) {
	ctx := context.Background()
	values, teardown := azuretest.Config(t)
	t.Cleanup(teardown)
	values["type"] = "azure"

	provisioner, err := New(ctx, scope.Literal(values))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pub, err := provisioner.PrepareKey(ctx, "ranke-db-signing")
	if err != nil {
		t.Fatalf("PrepareKey: %v", err)
	}
	t.Log("▸ created a P-256 key in the vault as ranke-db-signing")

	values["key"] = "ranke-db-signing"
	s, err := New(ctx, scope.Literal(values))
	if err != nil {
		t.Fatalf("New(with key): %v", err)
	}
	got, err := s.Public(ctx)
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if !got.(*ecdsa.PublicKey).Equal(pub.(*ecdsa.PublicKey)) {
		t.Fatal("the published key is not the one the vault holds")
	}
	t.Log("▸ a fresh signer read the key's public half")

	digest := sha256.Sum256([]byte("ranke-db azure signer"))
	sig, err := s.Sign(ctx, digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ecdsa.VerifyASN1(got.(*ecdsa.PublicKey), digest[:], sig) {
		t.Fatal("signature did not verify")
	}
	t.Log("▸ the vault's signature verifies under that key")

	if _, err := s.Sign(ctx, []byte("not a digest")); err == nil {
		t.Error("Sign(short input): want error")
	}
	t.Log("▸ ES256 refuses an input that is not a SHA-256 digest — OK")
}

// TestSignsWithTheStoredKey covers the other identity: an Ed25519 key the vault
// holds as a secret, fetched and signed with in process.
func TestSignsWithTheStoredKey(t *testing.T) {
	ctx := context.Background()
	values, teardown := azuretest.Config(t)
	t.Cleanup(teardown)
	values["type"] = "azure"
	values["secret"] = "ranke-db-signing-pem"

	provisioner, err := New(ctx, scope.Literal(values))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pub, err := provisioner.PrepareKey(ctx, "ranke-db-signing-pem")
	if err != nil {
		t.Fatalf("PrepareKey: %v", err)
	}
	t.Log("▸ minted an ed25519 key and wrote it to the vault as ranke-db-signing-pem")

	s, err := New(ctx, scope.Literal(values))
	if err != nil {
		t.Fatalf("New(with secret): %v", err)
	}
	got, err := s.Public(ctx)
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if !got.(ed25519.PublicKey).Equal(pub.(ed25519.PublicKey)) {
		t.Fatal("the fetched key is not the one the vault holds")
	}
	t.Log("▸ a fresh signer fetched the stored key")

	digest := sha256.Sum256([]byte("ranke-db azure signer"))
	sig, err := s.Sign(ctx, digest[:])
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if !ed25519.Verify(got.(ed25519.PublicKey), digest[:], sig) {
		t.Fatal("signature did not verify")
	}
	t.Log("▸ its signature verifies under that key")
}

// TestRejectsUnusableIdentities covers what a deployment gets wrong: no identity
// named at all, and one naming something the vault does not hold.
func TestRejectsUnusableIdentities(t *testing.T) {
	ctx := context.Background()
	values, teardown := azuretest.Config(t)
	t.Cleanup(teardown)
	values["type"] = "azure"

	unnamed, err := New(ctx, scope.Literal(values))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := unnamed.Public(ctx); err == nil {
		t.Error("Public without an identity: want error")
	}

	for field, name := range map[string]string{"key": "absent-key", "secret": "absent-secret"} {
		cfg := map[string]string{}
		for k, v := range values {
			cfg[k] = v
		}
		cfg[field] = name
		s, err := New(ctx, scope.Literal(cfg))
		if err != nil {
			t.Fatalf("New(%s): %v", field, err)
		}
		if _, err := s.Public(ctx); err == nil {
			t.Errorf("Public with an absent %s: want error", field)
		}
	}
	t.Log("▸ an identity the vault does not hold is an error at first use — OK")
}
