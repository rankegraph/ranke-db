// package: azure / crypto
// type:    logic
// job:     sign with an ed25519 key the vault holds as a secret, fetched once per run
// limits:  the signing happens in process (-> adapters/signer/inmemory)
//
// Key Vault publishes no Ed25519 key type, so an identity that must be Ed25519 is
// stored here as a secret and signed with in the server.
package azure

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/rankegraph/ranke-db/adapters/signer/inmemory"
	vault "github.com/rankegraph/ranke-db/adapters/vault/azure"
	"github.com/rankegraph/ranke-db/config/scope"
)

// secretMode signs with an ed25519 key the vault holds as a secret.
type secretMode struct {
	client *azsecrets.Client
	ref    string // "name", or "name/version"

	mu  sync.Mutex
	key *inmemory.Signer
}

// newSecret builds the secrets client and records which secret holds the key. It
// fetches nothing, so PrepareKey can write the key afterwards.
func newSecret(ctx context.Context, cfg scope.Section) (*secretMode, error) {
	client, err := vault.Client(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: %w", err)
	}
	ref, err := cfg.Get(ctx, "secret")
	if err != nil {
		return nil, fmt.Errorf("signer/azure: secret: %w", err)
	}
	return &secretMode{client: client, ref: ref}, nil
}

// Sign returns the ed25519 signature over hash.
func (s *secretMode) Sign(ctx context.Context, hash []byte) ([]byte, error) {
	key, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return key.Sign(ctx, hash)
}

// Public returns the ed25519 public key the signatures bind to.
func (s *secretMode) Public(ctx context.Context) (crypto.PublicKey, error) {
	key, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return key.Public(ctx)
}

// PrepareKey satisfies the conformance suite: it mints an ed25519 key, writes it to
// the vault as the secret name, and pins this signer to it.
func (s *secretMode) PrepareKey(ctx context.Context, name string) (crypto.PublicKey, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: generate ed25519: %w", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: marshal PKCS#8: %w", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	if _, err := s.client.SetSecret(ctx, name, azsecrets.SetSecretParameters{Value: &keyPEM}, nil); err != nil {
		return nil, fmt.Errorf("signer/azure: write secret %q: %w", name, err)
	}
	key, err := inmemory.New(ctx, scope.Literal(map[string]string{"key": keyPEM}))
	if err != nil {
		return nil, fmt.Errorf("signer/azure: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ref, s.key = name, key
	return key.Public(ctx)
}

// load fetches the secret once and keeps the key: the identity a server merges
// under is fixed for its run, so a rotated secret is picked up by restarting it.
func (s *secretMode) load(ctx context.Context) (*inmemory.Signer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != nil {
		return s.key, nil
	}
	if s.ref == "" {
		return nil, fmt.Errorf("signer/azure: no secret configured (name the secret holding the ed25519 PKCS#8 PEM)")
	}
	keyPEM, err := vault.NewWith(s.client).Secret(ctx, s.ref)
	if err != nil {
		return nil, err
	}
	key, err := inmemory.New(ctx, scope.Literal(map[string]string{"key": keyPEM}))
	if err != nil {
		return nil, fmt.Errorf("signer/azure: secret %q: %w", s.ref, err)
	}
	s.key = key
	return key, nil
}
