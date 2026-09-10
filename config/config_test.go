package config

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/internal/core"
)

// testKeyPEM generates a throwaway Ed25519 private key as PKCS#8 PEM. Tests
// legitimately fabricate a key to exercise loading; production keys are always
// provided by config, never generated.
func testKeyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// testFounderPEM is a first contributor's public key in the PEM form sequencer.founder
// takes. Only the public half ever reaches a config: the archive is founded under it,
// and whoever contributes keeps the rest.
func testFounderPEM(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate founding key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal founding key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// TestBuildResolvesAndWires loads a config whose signer key is delegated to an env
// var and asserts Run resolves it into a working shared signer. With no endpoints,
// the stack carries just the shared driven ports.
func TestBuildResolvesAndWires(t *testing.T) {
	t.Setenv("RANKE_TEST_SIGNER_KEY", testKeyPEM(t))

	const cfgJSON = `{
		"signer": {"type": "inmemory", "key": "env(RANKE_TEST_SIGNER_KEY)"}
	}`

	app, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if app.Signer == nil {
		t.Fatal("nil signer")
	}
	pub, err := app.Signer.Public(context.Background())
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	if _, ok := pub.(ed25519.PublicKey); !ok {
		t.Fatalf("signer public key = %T, want ed25519.PublicKey", pub)
	}
	digest := make([]byte, 32)
	if _, err := app.Signer.Sign(context.Background(), digest); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if len(app.Endpoints) != 0 {
		t.Fatalf("endpoints = %d, want 0", len(app.Endpoints))
	}
}

// TestBuildEndpointAdmits proves the per-endpoint pipeline: an admitted account is
// authenticated by the endpoint's NoAuth backend, authorized by its grants, and
// reaches the (stubbed) execute stage.
func TestBuildEndpointAdmits(t *testing.T) {
	const cfgJSON = `{
		"accounts": {
			"webapp": {"grants": ["R proj_*"]},
			"admin":  {"grants": ["C mgmt_*"]}
		},
		"endpoints": [{
			"transport": {"type": "rest"},
			"auth":      [{"type": "noauth", "subject": "webapp"}],
			"admit":     ["webapp"]
		}]
	}`
	c, err := load(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cr, err := c.buildEndpoint(context.Background(), c.Endpoints[0], &App{})
	if err != nil {
		t.Fatalf("buildEndpoint: %v", err)
	}

	req := &core.Request{Op: core.OpClaimQuery, Branch: "proj_x"}
	if _, err := cr.Handle(context.Background(), req); !errors.Is(err, core.ErrNotImplemented) {
		t.Fatalf("Handle = %v, want ErrNotImplemented (authorized, reached execute)", err)
	}
	if req.Principal.Account != "webapp" {
		t.Fatalf("account = %q, want webapp", req.Principal.Account)
	}
}

// TestBuildEndpointRejectsUnadmitted proves the admission boundary: an account
// that exists globally but is not admitted here is denied, because this endpoint's
// checker was built from the admitted subset only.
func TestBuildEndpointRejectsUnadmitted(t *testing.T) {
	const cfgJSON = `{
		"accounts": {
			"webapp": {"grants": ["R proj_*"]},
			"admin":  {"grants": ["C mgmt_*"]}
		},
		"endpoints": [{
			"transport": {"type": "rest"},
			"auth":      [{"type": "noauth", "subject": "admin"}],
			"admit":     ["webapp"]
		}]
	}`
	c, err := load(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cr, err := c.buildEndpoint(context.Background(), c.Endpoints[0], &App{})
	if err != nil {
		t.Fatalf("buildEndpoint: %v", err)
	}

	req := &core.Request{Op: core.OpClaimQuery, Branch: "proj_x"}
	if _, err := cr.Handle(context.Background(), req); !errors.Is(err, core.ErrForbidden) {
		t.Fatalf("Handle = %v, want ErrForbidden (admin authenticates but is not admitted here)", err)
	}
}

// TestBuildEndpointAPIKey drives the full per-endpoint path with a real credential
// scheme: an apikey backend, admitted account, and a key routed by scheme through
// core.Handle — authenticated, authorized, reaching the execute stub. A wrong key
// is rejected before authorization.
func TestBuildEndpointAPIKey(t *testing.T) {
	const key = "webapp-key-0123456789"
	sum := sha256.Sum256([]byte(key))
	cfgJSON := `{
		"accounts": {"webapp": {"grants": ["R proj_*"]}},
		"endpoints": [{
			"transport": {"type": "rest"},
			"auth":      [{"type": "apikey", "keys": [{"account": "webapp", "sha256": "` + hex.EncodeToString(sum[:]) + `"}]}],
			"admit":     ["webapp"]
		}]
	}`
	c, err := load(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	cr, err := c.buildEndpoint(context.Background(), c.Endpoints[0], &App{})
	if err != nil {
		t.Fatalf("buildEndpoint: %v", err)
	}

	ok := &core.Request{Op: core.OpClaimQuery, Branch: "proj_x", Credential: auth.Credential{Scheme: "apikey", Token: key}}
	if _, err := cr.Handle(context.Background(), ok); !errors.Is(err, core.ErrNotImplemented) {
		t.Fatalf("valid key: Handle = %v, want ErrNotImplemented (authenticated + authorized)", err)
	}
	if ok.Principal.Account != "webapp" {
		t.Fatalf("account = %q, want webapp", ok.Principal.Account)
	}

	bad := &core.Request{Op: core.OpClaimQuery, Branch: "proj_x", Credential: auth.Credential{Scheme: "apikey", Token: "wrong-key-0123456789"}}
	if _, err := cr.Handle(context.Background(), bad); err == nil || errors.Is(err, core.ErrNotImplemented) {
		t.Fatalf("wrong key: Handle = %v, want an auth error before authorization", err)
	}
}

// TestBuildEndpointAPIKeyRejectsBadDigest asserts apikey.New's validation surfaces
// through assembly: a malformed sha256 fails the endpoint build.
func TestBuildEndpointAPIKeyRejectsBadDigest(t *testing.T) {
	const cfgJSON = `{
		"accounts": {"webapp": {"grants": ["R proj_*"]}},
		"endpoints": [{
			"transport": {"type": "rest"},
			"auth":      [{"type": "apikey", "keys": [{"account": "webapp", "sha256": "not-hex"}]}],
			"admit":     ["webapp"]
		}]
	}`
	c, err := load(strings.NewReader(cfgJSON))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if _, err := c.buildEndpoint(context.Background(), c.Endpoints[0], &App{}); err == nil {
		t.Fatal("malformed sha256: want buildEndpoint error")
	}
}

// TestBuildMissingEnvFails asserts an unset env() delegation fails Run loud rather
// than yielding an empty key.
func TestBuildMissingEnvFails(t *testing.T) {
	const cfgJSON = `{"signer": {"type": "inmemory", "key": "env(RANKE_TEST_ABSENT)"}}`
	if _, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, false); err == nil {
		t.Fatal("Run succeeded with an unset env delegation; want error")
	}
}

// TestDevWiresSteerableClock asserts dev=true against sequencer.type "dev" mints the
// launch's steerable clock and hands it to the sequencer — the wiring POST /dev/clock
// later reaches through Core.
func TestDevWiresSteerableClock(t *testing.T) {
	t.Setenv("RANKE_TEST_SIGNER_KEY", testKeyPEM(t))
	t.Setenv("RANKE_TEST_FOUNDER", testFounderPEM(t))
	const cfgJSON = `{
		"signer": {"type": "inmemory", "key": "env(RANKE_TEST_SIGNER_KEY)"},
		"storage": {"type": "stack", "layers": [{"type": "mem"}]},
		"sequencer": {"type": "dev", "seed": "test-archive",
			"found": {"pubkey": "env(RANKE_TEST_FOUNDER)", "branch": "main"}}
	}`
	app, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, true)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if app.DevClock == nil {
		t.Fatal("dev=true against sequencer.type \"dev\": DevClock is nil")
	}
	// A fixture generator's story is typically dated in the past relative to whenever
	// it's re-run (SteerableClock's own tests pin why that must still land exactly).
	at := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	if got := app.DevClock.Advance(at); !got.Equal(at) {
		t.Errorf("Advance(%s) = %s, want the requested instant", at, got)
	}
}

// TestDevRequiresDevSequencer asserts dev=true refuses a concurrent (production)
// Sequencer rather than silently wiring a steerable clock into it — the whole point
// of R-C2DATE is that a production merge's witnessed time is real.
func TestDevRequiresDevSequencer(t *testing.T) {
	t.Setenv("RANKE_TEST_SIGNER_KEY", testKeyPEM(t))
	const cfgJSON = `{
		"signer": {"type": "inmemory", "key": "env(RANKE_TEST_SIGNER_KEY)"},
		"storage": {"type": "stack", "layers": [{"type": "mem"}]},
		"sequencer": {"type": "concurrent", "seed": "test-archive", "found": {"branch": "main"}}
	}`
	if _, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, true); err == nil {
		t.Fatal("dev=true against sequencer.type \"concurrent\": want error, got none")
	}
}

// TestVerify exercises offline syntax checks: shape, malformed grants, and dangling
// admit references all fail without touching env; resolve additionally requires
// every reference to resolve.
func TestVerify(t *testing.T) {
	const good = `{"signer": {"type": "inmemory", "key": "env(RANKE_TEST_VERIFY_KEY)"}}`

	if err := Verify(context.Background(), strings.NewReader(good), nil, LevelSyntax); err != nil {
		t.Fatalf("Verify syntax: %v", err)
	}
	if err := Verify(context.Background(), strings.NewReader(`{"nope": {}}`), nil, LevelSyntax); err == nil {
		t.Fatal("Verify syntax accepted an unknown section")
	}
	if err := Verify(context.Background(), strings.NewReader(`{"accounts": {"a": {"grants": ["CX foo_*"]}}}`), nil, LevelSyntax); err == nil {
		t.Fatal("Verify syntax accepted a malformed grant")
	}
	const danglingAdmit = `{"accounts": {"webapp": {"grants": ["R proj_*"]}}, "endpoints": [{"transport": {"type": "rest"}, "auth": [], "admit": ["ghost"]}]}`
	if err := Verify(context.Background(), strings.NewReader(danglingAdmit), nil, LevelSyntax); err == nil {
		t.Fatal("Verify syntax accepted an admit naming an undefined account")
	}
	const bareSocketPath = `{"endpoints": [{"transport": {"type": "rest", "addr": "/run/rankedb.sock"}, "auth": [], "admit": []}]}`
	if err := Verify(context.Background(), strings.NewReader(bareSocketPath), nil, LevelSyntax); err == nil {
		t.Fatal(`Verify syntax accepted an addr plainly meant as a socket path but missing its "unix://" scheme`)
	}
	overlongSocketPath := `{"endpoints": [{"transport": {"type": "rest", "addr": "unix://` + strings.Repeat("a", 200) + `"}, "auth": [], "admit": []}]}`
	if err := Verify(context.Background(), strings.NewReader(overlongSocketPath), nil, LevelSyntax); err == nil {
		t.Fatal("Verify syntax accepted a socket path over the platform's sun_path limit")
	}
	const validSocketPath = `{"endpoints": [{"transport": {"type": "rest", "addr": "unix:///run/rankedb.sock"}, "auth": [], "admit": []}]}`
	if err := Verify(context.Background(), strings.NewReader(validSocketPath), nil, LevelSyntax); err != nil {
		t.Fatalf("Verify syntax rejected a well-formed socket path: %v", err)
	}
	if err := Verify(context.Background(), strings.NewReader(good), nil, LevelResolve); err == nil {
		t.Fatal("Verify resolve accepted an unset env reference")
	}
	t.Setenv("RANKE_TEST_VERIFY_KEY", "x")
	if err := Verify(context.Background(), strings.NewReader(good), nil, LevelResolve); err != nil {
		t.Fatalf("Verify resolve: %v", err)
	}
}

// TestVerifyConnect exercises the connect depth: a shape-valid config with an
// unknown backend passes syntax but fails connect, while a valid mem+signer config
// (no endpoints) assembles cleanly.
func TestVerifyConnect(t *testing.T) {
	t.Setenv("RANKE_TEST_CONNECT_KEY", testKeyPEM(t))
	const good = `{"signer": {"type": "inmemory", "key": "env(RANKE_TEST_CONNECT_KEY)"}, "storage": {"type": "mem"}}`
	if err := Verify(context.Background(), strings.NewReader(good), nil, LevelConnect); err != nil {
		t.Fatalf("Verify connect: %v", err)
	}

	const badBackend = `{"storage": {"type": "bogus"}}`
	if err := Verify(context.Background(), strings.NewReader(badBackend), nil, LevelSyntax); err != nil {
		t.Fatalf("Verify syntax rejected a shape-valid config: %v", err)
	}
	if err := Verify(context.Background(), strings.NewReader(badBackend), nil, LevelConnect); err == nil {
		t.Fatal("Verify connect accepted an unknown storage backend")
	}
}
