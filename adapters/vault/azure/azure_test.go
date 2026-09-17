package azure

import (
	"context"
	"strings"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/rankegraph/ranke-db/adapters/vault/azure/azuretest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// step narrates a phase of the test; visible under `go test -v`.
func step(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Logf("▸ "+format, args...)
}

// TestSecret seeds a Key Vault — the lowkey-vault emulator via podman, or the live
// vault RANKE_AZURE_VAULT_URL names — and asserts the adapter reads the secret back
// through the vault.Vault port, at its current version and at a pinned one. It
// skips when podman is unavailable so the offline gate stays green.
func TestSecret(t *testing.T) {
	ctx := context.Background()
	cfg, teardown := azuretest.Setup(t)
	t.Cleanup(teardown)

	client, err := Client(ctx, cfg)
	if err != nil {
		t.Fatalf("Client: %v", err)
	}

	step(t, "seeding secret ranke-signing")
	first := set(t, client, "ranke-signing", "first")
	set(t, client, "ranke-signing", "s3cr3t")

	v, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	step(t, "reading vault(ranke-signing) through the adapter")
	got, err := v.Secret(ctx, "ranke-signing")
	if err != nil {
		t.Fatalf("Secret: %v", err)
	}
	if got != "s3cr3t" {
		t.Fatalf("Secret = %q, want %q", got, "s3cr3t")
	}
	step(t, "adapter returned the current version intact — OK")

	step(t, "reading vault(ranke-signing/%s), the superseded version", first)
	got, err = v.Secret(ctx, "ranke-signing/"+first)
	if err != nil {
		t.Fatalf("Secret(pinned version): %v", err)
	}
	if got != "first" {
		t.Fatalf("Secret(pinned version) = %q, want %q", got, "first")
	}
	step(t, "a pinned version reads the value that version holds — OK")

	if _, err := v.Secret(ctx, "absent-secret"); err == nil {
		t.Fatal("Secret(absent-secret): want error")
	}
	step(t, "an absent secret is an error, not an empty string — OK")
}

// TestNewRequiresURL covers what construction rejects without reaching a vault,
// so it runs offline.
func TestNewRequiresURL(t *testing.T) {
	ctx := context.Background()
	bad := map[string]scope.Section{
		"no url":      scope.Literal(map[string]string{"type": "azure"}),
		"empty url":   scope.Literal(map[string]string{"type": "azure", "url": ""}),
		"bad ca_cert": scope.Literal(map[string]string{"type": "azure", "url": "https://v.vault.azure.net/", "ca_cert": "not-a-pem"}),
	}
	for name, cfg := range bad {
		if _, err := New(ctx, cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// set writes value under name and returns the version it landed on.
func set(t *testing.T, client *azsecrets.Client, name, value string) string {
	t.Helper()
	resp, err := client.SetSecret(context.Background(), name, azsecrets.SetSecretParameters{Value: &value}, nil)
	if err != nil {
		t.Fatalf("seed secret %q: %v", name, err)
	}
	if resp.ID == nil {
		t.Fatalf("seed secret %q: response carries no id", name)
	}
	version := resp.ID.Version()
	if strings.TrimSpace(version) == "" {
		t.Fatalf("seed secret %q: response carries no version", name)
	}
	return version
}
