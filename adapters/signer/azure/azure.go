// package: azure / crypto
// type:    adapter
// job:     sign merges under an Azure Key Vault identity, in either scheme V-SIGN names
// limits:  one identity per configuration — a vault key, or a secret (-> adapters/signer)
//
// Package azure is the Azure Key Vault signer backend, in the two shapes `V-SIGN`
// allows: "key" signs under ES256 in the vault (-> key.go), "secret" with an
// ed25519 key the vault holds (-> secret.go). The connection is the Key Vault
// secret backend's (-> adapters/vault/azure), so one section serves both ports.
package azure

import (
	"context"
	"crypto"
	"fmt"
	"strings"

	"github.com/rankegraph/ranke-db/config/scope"
)

// Signer signs under an Azure Key Vault identity. It implements signer.Signer (and
// the private signer test view via PrepareKey).
type Signer struct{ mode }

// mode is one of the two identities: the port's pair plus the conformance hook,
// which provisions the key the suite then signs with.
type mode interface {
	Sign(ctx context.Context, hash []byte) ([]byte, error)
	Public(ctx context.Context) (crypto.PublicKey, error)
	PrepareKey(ctx context.Context, name string) (crypto.PublicKey, error)
}

// New builds the backend from "url", the credential fields, and one of "key" or
// "secret" — either as "name" or "name/version". Naming neither leaves a key signer
// for a test to provision; naming both is refused.
func New(ctx context.Context, cfg scope.Section) (*Signer, error) {
	if cfg.HasValue("key") && cfg.HasValue("secret") {
		return nil, fmt.Errorf(`signer/azure: "key" and "secret" name two identities; configure one`)
	}
	if cfg.HasValue("secret") {
		m, err := newSecret(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return &Signer{mode: m}, nil
	}
	m, err := newKey(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &Signer{mode: m}, nil
}

// splitRef splits "name/version" on the last '/'; a ref without one names the
// current version, which Key Vault denotes by the empty string.
func splitRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, "/"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}
