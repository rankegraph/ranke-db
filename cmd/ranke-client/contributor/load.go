// package: contributor / cmd
// type:    logic
// job:     resolve a --signing-key argument to the contributor key it names
// limits:  a seam over keysource, ParseKeypair and the signer port; the grammars are theirs
// (-> github.com/rankegraph/ranke-go/keysource, adapters/signer)
//
// A contributor key is application-held: it signs claims into their ids and never reaches
// a server. WithTTY is granted here because this is a tool a person runs — a server must
// not, or it can be stopped on a terminal read. A key named as a vault key signs in that
// vault instead, so the machine running this holds no key material at all.
package contributor

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/rankegraph/ranke-go"
	"github.com/rankegraph/ranke-go/keysource"

	"github.com/rankegraph/ranke-db/adapters/signer"
	cfgscope "github.com/rankegraph/ranke-db/config/scope"
)

// azureScheme prefixes a key the vault holds and signs with, rather than one this machine
// reads.
const azureScheme = "azure:"

// Load resolves spec to the keypair it names: "azure:<key URL>" signs through Azure Key
// Vault, and a path, file:PATH, env:NAME, stdin or prompt reads the key here. in is where
// "stdin" reads from; ctx bounds a signature a vault makes.
func Load(ctx context.Context, spec string, in io.Reader) (ranke.Keypair, error) {
	if rest, found := strings.CutPrefix(spec, azureScheme); found {
		return vaultKeypair(ctx, rest)
	}
	pemBytes, err := keysource.Load(spec, in, keysource.WithTTY())
	if err != nil {
		return ranke.Keypair{}, err
	}
	return ranke.ParseKeypair(pemBytes)
}

// vaultKeypair builds a keypair over an Azure Key Vault key, signing through the vault.
// Credentials are the ambient identity's — a managed identity, the AZURE_* variables, a
// signed-in az — so the spec carries a location and no secret.
func vaultKeypair(ctx context.Context, keyURL string) (ranke.Keypair, error) {
	vault, key, err := splitKeyURL(keyURL)
	if err != nil {
		return ranke.Keypair{}, err
	}
	sig, err := signer.New(ctx, cfgscope.Literal(map[string]string{
		"type": "azure", "url": vault, "key": key,
	}))
	if err != nil {
		return ranke.Keypair{}, err
	}
	private, err := signer.CryptoSigner(ctx, sig)
	if err != nil {
		return ranke.Keypair{}, err
	}
	pubkey, err := ranke.EncodePublicKey(private.Public())
	if err != nil {
		return ranke.Keypair{}, fmt.Errorf("signing key %s: encode public key: %w", keyURL, err)
	}
	return ranke.Keypair{Private: private, Pubkey: pubkey}, nil
}

// splitKeyURL reads https://VAULT/keys/NAME — a Key Vault key's own URL, as the portal and
// the CLI print it — into the vault it lives in and the key within it. A trailing version
// is kept on the key, which is how the backend pins one.
func splitKeyURL(raw string) (vault, key string, err error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Scheme == "" {
		return "", "", fmt.Errorf("signing key %q: want a key URL, https://VAULT/keys/NAME", raw)
	}
	name := strings.TrimPrefix(strings.TrimPrefix(parsed.Path, "/"), "keys/")
	if name == "" || name == strings.TrimPrefix(parsed.Path, "/") {
		return "", "", fmt.Errorf("signing key %q: the path names no key, want /keys/NAME", raw)
	}
	return parsed.Scheme + "://" + parsed.Host + "/", strings.TrimSuffix(name, "/"), nil
}
