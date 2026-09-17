// package: azure / secrets
// type:    adapter
// job:     resolve vault(ref) secrets from Azure Key Vault
// limits:  secrets only; Key Vault keys sign nothing Ranke accepts (-> adapters/vault, V-SIGN)
//
// Package azure is the Azure Key Vault secret backend. A ref is "name", or
// "name/version" to pin a version. Connect is exported because the Azure signer
// reads one section's connection to sign in the same vault (-> adapters/signer/azure).
package azure

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"

	"github.com/rankegraph/ranke-db/config/scope"
)

// Vault reads secrets from an Azure Key Vault.
type Vault struct {
	client *azsecrets.Client
}

// New builds the Azure Key Vault backend from the vault section (-> Client).
func New(ctx context.Context, cfg scope.Section) (*Vault, error) {
	client, err := Client(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return NewWith(client), nil
}

// NewWith wraps a client built elsewhere, so a caller holding one already — the
// Azure signer, whose key is a secret in the same vault — reads through the same
// refs and the same errors.
func NewWith(client *azsecrets.Client) *Vault {
	return &Vault{client: client}
}

// Connection is what a Key Vault client is built from. The signer builds a keys
// client from the same fields, so one section configures both ports.
type Connection struct {
	URL        string
	Credential azcore.TokenCredential
	Options    azcore.ClientOptions

	// ChallengeResourceUnverified holds where the section pins a certificate: an
	// emulator answers the authentication challenge under its own host.
	ChallengeResourceUnverified bool
}

// Connect reads "url" (the vault URI, required), the optional service principal
// "tenant_id"/"client_id"/"client_secret" (absent it, the ambient identity
// answers), and the optional "ca_cert", a PEM certificate trusted in place of the
// system roots — a private CA, or a local emulator.
func Connect(ctx context.Context, cfg scope.Section) (Connection, error) {
	url, err := cfg.Get(ctx, "url")
	if err != nil {
		return Connection{}, fmt.Errorf("azure: url: %w", err)
	}
	if url == "" {
		return Connection{}, fmt.Errorf("azure: url is required (the Key Vault URI)")
	}
	options, err := clientOptions(ctx, cfg)
	if err != nil {
		return Connection{}, err
	}
	cred, err := credential(ctx, cfg, options)
	if err != nil {
		return Connection{}, err
	}
	return Connection{URL: url, Credential: cred, Options: options, ChallengeResourceUnverified: cfg.HasValue("ca_cert")}, nil
}

// Client builds a Key Vault secrets client from the section (-> Connect).
func Client(ctx context.Context, cfg scope.Section) (*azsecrets.Client, error) {
	conn, err := Connect(ctx, cfg)
	if err != nil {
		return nil, err
	}
	client, err := azsecrets.NewClient(conn.URL, conn.Credential, &azsecrets.ClientOptions{
		ClientOptions:                        conn.Options,
		DisableChallengeResourceVerification: conn.ChallengeResourceUnverified,
	})
	if err != nil {
		return nil, fmt.Errorf("azure: client: %w", err)
	}
	return client, nil
}

// Secret reads ref ("name" or "name/version") from the vault and returns the
// secret's value.
func (v *Vault) Secret(ctx context.Context, ref string) (string, error) {
	name, version := splitRef(ref)
	if name == "" {
		return "", fmt.Errorf("vault/azure: ref %q names no secret", ref)
	}
	resp, err := v.client.GetSecret(ctx, name, version, nil)
	if err != nil {
		return "", fmt.Errorf("vault/azure: read %q: %w", name, err)
	}
	if resp.Value == nil {
		return "", fmt.Errorf("vault/azure: secret %q has no value", name)
	}
	return *resp.Value, nil
}

// splitRef splits "name/version" on the last '/'; a ref without one names the
// secret's current version, which Key Vault denotes by the empty string.
func splitRef(ref string) (name, version string) {
	if i := strings.LastIndex(ref, "/"); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return ref, ""
}

// credential builds the service principal the section names, or falls back to the
// ambient identity.
func credential(ctx context.Context, cfg scope.Section, options azcore.ClientOptions) (azcore.TokenCredential, error) {
	if !cfg.HasValue("client_secret") {
		cred, err := azidentity.NewDefaultAzureCredential(&azidentity.DefaultAzureCredentialOptions{ClientOptions: options})
		if err != nil {
			return nil, fmt.Errorf("azure: default credential: %w", err)
		}
		return cred, nil
	}
	tenant, err := cfg.Get(ctx, "tenant_id")
	if err != nil {
		return nil, fmt.Errorf("azure: tenant_id: %w (required beside client_secret)", err)
	}
	clientID, err := cfg.Get(ctx, "client_id")
	if err != nil {
		return nil, fmt.Errorf("azure: client_id: %w (required beside client_secret)", err)
	}
	secret, err := cfg.Get(ctx, "client_secret")
	if err != nil {
		return nil, fmt.Errorf("azure: client_secret: %w", err)
	}
	cred, err := azidentity.NewClientSecretCredential(tenant, clientID, secret, &azidentity.ClientSecretCredentialOptions{ClientOptions: options})
	if err != nil {
		return nil, fmt.Errorf("azure: client secret credential: %w", err)
	}
	return cred, nil
}

// clientOptions carries "ca_cert" into the transport the credential shares with
// the vault client, so the token endpoint is reached under it too.
func clientOptions(ctx context.Context, cfg scope.Section) (azcore.ClientOptions, error) {
	if !cfg.HasValue("ca_cert") {
		return azcore.ClientOptions{}, nil
	}
	caPEM, err := cfg.Get(ctx, "ca_cert")
	if err != nil {
		return azcore.ClientOptions{}, fmt.Errorf("azure: ca_cert: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		return azcore.ClientOptions{}, fmt.Errorf("azure: ca_cert holds no PEM certificate")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return azcore.ClientOptions{Transport: &http.Client{Transport: transport}}, nil
}
