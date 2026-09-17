// package: azuretest / crypto
// type:    test-support
// job:     the Azure Key Vault signer's conformance setup hooks, one per identity
// limits:  a test helper; the vault comes from the secret backend's hook
// (-> adapters/vault/azure/azuretest)
//
// Package azuretest is the Azure-specific setup for the signer conformance suite,
// one hook per identity the backend serves. The counterpart is the Key Vault the
// secret backend is tested against: this stamps the signer's section onto that
// connection and leaves the key to the suite, which provisions it through
// PrepareKey.
package azuretest

import (
	"testing"

	"github.com/rankegraph/ranke-db/adapters/vault/azure/azuretest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// Setup returns the conformance config for the vault-signing identity: a Key Vault
// key under ES256. It skips when neither a live Key Vault nor podman is available.
func Setup(t *testing.T) (scope.Section, func()) {
	t.Helper()
	return section(t, "key")
}

// SetupSecret is Setup for the other identity the backend serves: an Ed25519 key
// the vault holds as a secret.
func SetupSecret(t *testing.T) (scope.Section, func()) {
	t.Helper()
	return section(t, "secret")
}

// section names the identity under field, as the name the suite prepares.
func section(t *testing.T, field string) (scope.Section, func()) {
	t.Helper()
	values, teardown := azuretest.Config(t)
	values["type"] = "azure"
	values[field] = "conformance"
	return scope.Literal(values), teardown
}
