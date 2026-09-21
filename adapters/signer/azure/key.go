// package: azure / crypto
// type:    logic
// job:     sign through a Key Vault key under ES256, the private half never leaving Azure
// limits:  P-256 alone, the curve V-SIGN names (-> adapters/signer/azure)
//
// Key Vault signs a digest and answers the raw r||s pair; a Go crypto.Signer answers
// ASN.1 DER for ECDSA and go-cose converts it back, so DER is what the port hands on.
package azure

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/asn1"
	"fmt"
	"math/big"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"

	vault "github.com/rankegraph/ranke-db/adapters/vault/azure"
	"github.com/rankegraph/ranke-db/config/scope"
)

// keyMode signs through a Key Vault key.
type keyMode struct {
	client  *azkeys.Client
	name    string
	version string

	mu  sync.Mutex
	pub *ecdsa.PublicKey
}

// resolve reads the key once, holding its public half and the version that answered:
// the version a server signs under is then fixed for its run, so a rotation reaches
// it at the next launch rather than mid-merge.
func (k *keyMode) resolve(ctx context.Context) (*ecdsa.PublicKey, string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.pub != nil {
		return k.pub, k.version, nil
	}
	if k.name == "" {
		return nil, "", fmt.Errorf("signer/azure: no key configured (name the Key Vault key that signs)")
	}
	resp, err := k.client.GetKey(ctx, k.name, k.version, nil)
	if err != nil {
		return nil, "", fmt.Errorf("signer/azure: read key %q: %w", k.name, err)
	}
	pub, err := publicKey(resp.Key)
	if err != nil {
		return nil, "", err
	}
	if resp.Key.KID == nil {
		return nil, "", fmt.Errorf("signer/azure: key %q came back without an identifier", k.name)
	}
	k.pub, k.version = pub, resp.Key.KID.Version()
	return pub, k.version, nil
}

// newKey builds the keys client and records which key signs. It reads nothing, so
// PrepareKey can create the key afterwards.
func newKey(ctx context.Context, cfg scope.Section) (*keyMode, error) {
	conn, err := vault.Connect(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: %w", err)
	}
	client, err := azkeys.NewClient(conn.URL, conn.Credential, &azkeys.ClientOptions{
		ClientOptions:                        conn.Options,
		DisableChallengeResourceVerification: conn.ChallengeResourceUnverified,
	})
	if err != nil {
		return nil, fmt.Errorf("signer/azure: keys client: %w", err)
	}
	m := &keyMode{client: client}
	if cfg.HasValue("key") {
		ref, err := cfg.Get(ctx, "key")
		if err != nil {
			return nil, fmt.Errorf("signer/azure: key: %w", err)
		}
		m.name, m.version = splitRef(ref)
	}
	return m, nil
}

// Sign has the vault sign hash under ES256 and returns the signature as ASN.1 DER.
// ES256 signs a SHA-256 digest, which is the length Key Vault accepts.
func (k *keyMode) Sign(ctx context.Context, hash []byte) ([]byte, error) {
	if len(hash) != sha256.Size {
		return nil, fmt.Errorf("signer/azure: ES256 signs a %d-byte SHA-256 digest, got %d bytes", sha256.Size, len(hash))
	}
	_, version, err := k.resolve(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := k.client.Sign(ctx, k.name, version, azkeys.SignParameters{
		Algorithm: to.Ptr(azkeys.SignatureAlgorithmES256),
		Value:     hash,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: sign with %q: %w", k.name, err)
	}
	return derSignature(resp.Result)
}

// Public returns the public half of the key the vault signs with.
func (k *keyMode) Public(ctx context.Context) (crypto.PublicKey, error) {
	pub, _, err := k.resolve(ctx)
	if err != nil {
		return nil, err
	}
	return pub, nil
}

// PrepareKey satisfies the conformance suite: it creates a P-256 key in the vault
// and pins this signer to it. Provisioning, never a launch path.
func (k *keyMode) PrepareKey(ctx context.Context, name string) (crypto.PublicKey, error) {
	resp, err := k.client.CreateKey(ctx, name, azkeys.CreateKeyParameters{
		Kty:    to.Ptr(azkeys.KeyTypeEC),
		Curve:  to.Ptr(azkeys.CurveNameP256),
		KeyOps: []*azkeys.KeyOperation{to.Ptr(azkeys.KeyOperationSign), to.Ptr(azkeys.KeyOperationVerify)},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("signer/azure: create key %q: %w", name, err)
	}
	pub, err := publicKey(resp.Key)
	if err != nil {
		return nil, err
	}
	if resp.Key.KID == nil {
		return nil, fmt.Errorf("signer/azure: key %q came back without an identifier", name)
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.name, k.version, k.pub = name, resp.Key.KID.Version(), pub
	return pub, nil
}

// publicKey reads a P-256 public key out of the JWK the vault answers with.
func publicKey(jwk *azkeys.JSONWebKey) (*ecdsa.PublicKey, error) {
	if jwk == nil || jwk.Crv == nil || jwk.X == nil || jwk.Y == nil {
		return nil, fmt.Errorf("signer/azure: the vault answered no EC public key")
	}
	if *jwk.Crv != azkeys.CurveNameP256 {
		return nil, fmt.Errorf("signer/azure: key is on curve %q, and V-SIGN names P-256", *jwk.Crv)
	}
	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(jwk.X),
		Y:     new(big.Int).SetBytes(jwk.Y),
	}, nil
}

// derSignature converts Key Vault's raw r||s pair into the ASN.1 DER a Go
// crypto.Signer answers with.
func derSignature(raw []byte) ([]byte, error) {
	const size = 32 // P-256 coordinates, left-padded by the vault
	if len(raw) != 2*size {
		return nil, fmt.Errorf("signer/azure: the vault answered a %d-byte signature, want %d", len(raw), 2*size)
	}
	der, err := asn1.Marshal(struct{ R, S *big.Int }{
		R: new(big.Int).SetBytes(raw[:size]),
		S: new(big.Int).SetBytes(raw[size:]),
	})
	if err != nil {
		return nil, fmt.Errorf("signer/azure: encode signature: %w", err)
	}
	return der, nil
}
