// package: azuretest / secrets
// type:    test-support
// job:     stand up a Key Vault the Azure backends can be driven against — an emulator, or a live vault
// limits:  a test helper; it skips without podman (-> adapters/vault/azure, tools/podman)
//
// Package azuretest is the Azure backends' counterpart setup, shared by the vault
// test and the signer's conformance hook. It runs lowkey-vault, a Key Vault test
// double, via podman: HTTPS under a self-signed certificate handed on as
// "ca_cert", plus the token endpoint the credential reads through
// IDENTITY_ENDPOINT. Setting RANKE_AZURE_VAULT_URL runs the same tests against a
// live Key Vault under the identity the environment already holds.
package azuretest

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/tools/podman"
)

// image is the pinned lowkey-vault release. Its Key Vault API version moves with
// the tag, so a bump is a deliberate step, never "latest" drifting under a test.
const image = "docker.io/nagyesta/lowkey-vault:7.3.98-ubi10-minimal"

// liveURL names the environment variable that points the Azure tests at a real
// Key Vault.
const liveURL = "RANKE_AZURE_VAULT_URL"

// Setup returns the vault section for the Azure backend plus a teardown.
func Setup(t *testing.T) (scope.Section, func()) {
	t.Helper()
	values, teardown := Config(t)
	values["type"] = "azure"
	return scope.Literal(values), teardown
}

// Config is Setup's connection half — "url", and against the emulator "ca_cert" —
// for a caller stamping its own section on top, as the signer does with its key.
func Config(t *testing.T) (map[string]string, func()) {
	t.Helper()
	if url := os.Getenv(liveURL); url != "" {
		t.Logf("▸ using the live Key Vault at %s (%s is set)", url, liveURL)
		return map[string]string{"url": url}, func() {}
	}

	addr, more, teardown := podman.RunMore(t, podman.Spec{
		Image: image,
		Port:  8443,
		More:  []int{8080},
		// The published host port is not 8443, and a vault is registered under the
		// port it was started on; relaxed ports match it on host alone.
		Env: map[string]string{"LOWKEY_VAULT_RELAXED_PORTS": "true"},
	})

	url := "https://" + addr
	caPEM := serverCertificate(t, addr, teardown)
	waitReady(t, url, caPEM, teardown)

	// The credential chain reads a managed identity from these two; lowkey-vault
	// answers the token endpoint and ignores what the header holds.
	t.Setenv("IDENTITY_ENDPOINT", "http://"+more[8080]+"/metadata/identity/oauth2/token")
	t.Setenv("IDENTITY_HEADER", "lowkey-vault")

	return map[string]string{"url": url, "ca_cert": caPEM}, teardown
}

// serverCertificate reads the certificate the adapter is then told to trust off
// the handshake itself: no keystore to unpack, right for every image.
func serverCertificate(t *testing.T, addr string, teardown func()) string {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec // reading the certificate is the point
		if err == nil {
			defer func() { _ = conn.Close() }()
			cert := conn.ConnectionState().PeerCertificates[0]
			return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}))
		}
		if time.Now().After(deadline) {
			teardown()
			t.Fatalf("lowkey-vault did not serve TLS in time: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// waitReady polls the emulator's management API until the default vaults are
// registered — the port opens before the application answers.
func waitReady(t *testing.T, url, caPEM string, teardown func()) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: tlsConfig(t, caPEM)}}
	deadline := time.Now().Add(60 * time.Second)
	start := time.Now()
	for {
		resp, err := client.Get(url + "/management/vault") //nolint:noctx // a readiness poll with its own deadline
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				t.Logf("▸ lowkey-vault ready at %s after %s", url, time.Since(start).Round(100*time.Millisecond))
				return
			}
		}
		if time.Now().After(deadline) {
			teardown()
			t.Fatalf("lowkey-vault did not become ready in time: %v", err)
		}
		time.Sleep(300 * time.Millisecond)
	}
}

// tlsConfig trusts the emulator's own certificate and nothing else.
func tlsConfig(t *testing.T, caPEM string) *tls.Config {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(caPEM)) {
		t.Fatal("emulator certificate is not PEM")
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}
