// package: vault / secrets
// type:    interface + factory
// job:     the Vault port — a secret reference in, its value out — plus its factory
// limits:  contract + dispatch; secret fetching lives in the backends (-> adapters/vault/openbao, azure)
//
// Package vault defines the secret-resolution port and builds the configured
// backend. The config loader calls Secret to expand vault(ref) placeholders. It
// is optional: a stack whose every adapter needs no secret (mem/fs storage,
// in-memory signer, no auth) configures no vault at all. The vault is built
// env-only — secret-zero cannot resolve vault() before the vault exists.
package vault

import (
	"context"
	"fmt"

	"github.com/rankegraph/ranke-db/adapters/vault/azure"
	"github.com/rankegraph/ranke-db/adapters/vault/openbao"
	"github.com/rankegraph/ranke-db/config/scope"
)

// Vault resolves secret references to their plaintext values. Backends: OpenBao
// KV and Azure Key Vault (-> sub-packages).
type Vault interface {
	// Secret returns the value stored under ref, or an error if it is absent
	// or the vault is unreachable.
	Secret(ctx context.Context, ref string) (string, error)
}

// New builds the vault backend named by the section's "type": "openbao" (KV v2)
// or "azure" (Azure Key Vault secrets). An empty or unknown type is an error.
func New(ctx context.Context, cfg scope.Section) (Vault, error) {
	var t string
	if cfg.HasValue("type") {
		var err error
		if t, err = cfg.Get(ctx, "type"); err != nil {
			return nil, fmt.Errorf("vault: type: %w", err)
		}
	}
	switch t {
	case "openbao":
		return openbao.New(ctx, cfg)
	case "azure":
		return azure.New(ctx, cfg)
	case "":
		return nil, fmt.Errorf("vault: no backend type configured")
	default:
		return nil, fmt.Errorf("vault: unknown backend type %q", t)
	}
}
