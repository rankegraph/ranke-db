package rest_http

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	ranke "github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/adapters/storage"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// newTestServer builds the endpoint over a core with no driven ports: every request
// authenticates as "ops", and a routed one reaches execution and finds no archive (501).
// That 501 is what tells "routed" from "not routed" — a 404 is the mux's own.
func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	return newServerFor(t, nil, nil)
}

// everyReadRight is what the test subject holds unless a case narrows it: each reserved
// scope has to be granted by name, so the default names them all.
var everyReadRight = []string{"CR *", "C $branches", "R $universe", "R $archive", "R $branches"}

// newServerFor builds the endpoint over a core bound to the given driven ports.
func newServerFor(t *testing.T, seq sequencer.Sequencer, store storage.Storage) http.Handler {
	t.Helper()
	return newGrantedServer(t, everyReadRight, seq, store)
}

// newGrantedServer builds the endpoint over a core admitting one account with exactly the
// grants given, so a case can withhold one right and see what stops working.
func newGrantedServer(t *testing.T, grants []string, seq sequencer.Sequencer, store storage.Storage) http.Handler {
	t.Helper()
	return newServerWith(t, grants, map[string]string{"addr": ":0"}, seq, store)
}

// newServerWith builds the endpoint from a transport config of the case's choosing, so a
// case can declare things like allowedOrigins without a second copy of the wiring.
func newServerWith(
	t *testing.T,
	grants []string,
	transport map[string]string,
	seq sequencer.Sequencer,
	store storage.Storage,
) http.Handler {
	t.Helper()
	ctx := context.Background()

	a, err := auth.New(ctx, scope.Literal(map[string]string{"type": "noauth", "subject": "ops"}))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{"ops": grants})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	s, err := New(ctx, scope.Literal(transport), core.New(set, chk, seq, store))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.srv.Handler
}

// newServingServer builds the endpoint over a real stack — an in-memory universe, a dev
// sequencer, a real signer — for the routes whose behaviour needs an archive to exist.
func newServingServer(t *testing.T) http.Handler {
	t.Helper()
	h, _ := newServingStack(t, everyReadRight)
	return h
}

// newServingStack builds a serving endpoint and hands back the Universe behind it, so a
// test can put a claim where only the unconfined scope will find it.
func newServingStack(t *testing.T, grants []string) (http.Handler, ranke.Universe) {
	t.Helper()
	ctx := context.Background()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sig, err := signer.New(ctx, scope.Literal(map[string]string{"type": "inmemory", "key": string(key)}))
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}
	store := ranke.NewMemoryUniverse()
	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": t.Name()}), store, sig, nil)
	if err != nil {
		t.Fatalf("sequencer.New: %v", err)
	}
	foundArchive(t, seq)
	return newGrantedServer(t, grants, seq, store), store
}

// TestRoutes pins that every route the contract declares is actually reachable —
// the one thing a generated router can silently get wrong, and which no unit test on
// the mapping would catch. A routed request lands on the execute stub (501); an
// unrouted one is the mux's own 404, so the two are told apart by status.
func TestRoutes(t *testing.T) {
	h := newTestServer(t)
	id := testClaimID

	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/health", ""},
		{http.MethodPost, "/query", `{"select": {"branch": "foo"}}`},
		{http.MethodPost, "/contribute?branch=foo", ""},
		{http.MethodGet, "/branches", ""},
		{http.MethodGet, "/branches/foo/head", ""},
		{http.MethodGet, "/branches/foo/info", ""},
		{http.MethodGet, "/archive/info", ""},
		{http.MethodGet, "/branches/foo/claims/" + id, ""},
		{http.MethodGet, "/branches/foo/claims/" + id + "/content", ""},
		{http.MethodGet, "/archive/claims/" + id, ""},
		{http.MethodGet, "/archive/claims/" + id + "/content", ""},
		{http.MethodGet, "/universe/claims/" + id, ""},
		{http.MethodGet, "/universe/claims/" + id + "/content", ""},
		{http.MethodGet, "/system/layers", ""},
		{http.MethodGet, "/system/verifications", ""},
		{http.MethodPost, "/system/verifications", `{"closure": "foo"}`},
		{http.MethodGet, "/system/verifications/r1", ""},
		{http.MethodDelete, "/system/verifications/r1", ""},
		{http.MethodPost, "/system/verifications/r1/cancel", ""},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				r.Header.Set("Content-Type", "application/json")
			}
			h.ServeHTTP(rec, r)
			if rec.Code == http.StatusNotFound {
				t.Fatalf("status = 404, want the route to be reachable: %s", rec.Body.String())
			}
		})
	}
}

// TestQueryRejectsAtTheBoundary pins the two RankeQL rules the JSON schema cannot
// state. They are enforced before the request reaches core, so each is a 400 rather
// than the 501 a routed-but-unimplemented read returns.
func TestQueryRejectsAtTheBoundary(t *testing.T) {
	h := newTestServer(t)

	for _, tc := range []struct{ name, body string }{
		{"no scope", `{"select": {}}`},
		{"universe without a head", `{"select": {"branch": "$universe"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(rec, r)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestNoCypherRoute pins the contract's refusal to carry a client Cypher string: the
// route the pre-hexagonal API exposed must not resolve.
func TestNoCypherRoute(t *testing.T) {
	h := newTestServer(t)
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/branches/foo/gql", strings.NewReader(`{"query": "MATCH (n) RETURN n"}`))
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 — there is no client Cypher endpoint", rec.Code)
	}
}

// TestExplorerConfigGate pins config's "explorer" wiring rather than the embed itself —
// this build carries no explorer.html (-tags explorer is release-only), so every case
// 404s; what the two messages tell apart is whether config's own switch was consulted
// before ever reaching frontend.Explorer.
func TestExplorerConfigGate(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  map[string]string
		want string
	}{
		{"absent defaults to enabled", map[string]string{"addr": ":0"}, "not embedded"},
		{"true is enabled", map[string]string{"addr": ":0", "explorer": "true"}, "not embedded"},
		{"false is disabled", map[string]string{"addr": ":0", "explorer": "false"}, "disabled by config"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newServerWith(t, everyReadRight, tc.cfg, nil, nil)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/explorer", nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("body = %q, want it to mention %q", rec.Body.String(), tc.want)
			}
		})
	}
}

// TestVerificationHeaders pins the headers the contract promises on the run API: a 202
// names where to poll and how soon, and a running report repeats the hint. Without these
// a client has to guess a URL it was told it would be given.
func TestVerificationHeaders(t *testing.T) {
	h := newServingServer(t)

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/system/verifications", strings.NewReader(`{"closure":"$archive"}`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, r)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/system/verifications/") || loc == "/system/verifications/" {
		t.Fatalf("Location = %q, want the report's own route", loc)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After on 202 — a client is told to poll but not how soon")
	}

	// The Location must actually resolve.
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, loc, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200: %s", loc, get.Code, get.Body.String())
	}
}

// TestBusyCarriesRetryAfter pins that a refusal for want of a free slot says when to come
// back. A 429 without it tells a client to retry and leaves the interval to guesswork.
func TestBusyCarriesRetryAfter(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, core.CatBusy, "too many runs")

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After on 429")
	}

	// Other categories carry none: there is nothing to wait for.
	other := httptest.NewRecorder()
	writeError(other, core.CatNotFound, "nope")
	if got := other.Header().Get("Retry-After"); got != "" {
		t.Fatalf("Retry-After = %q on 404, want none", got)
	}
}

// TestParseAddr pins the "unix://" convention: it names a socket path, and anything
// else is read as a TCP address exactly as before.
func TestParseAddr(t *testing.T) {
	for _, tc := range []struct {
		name, addr, network, address string
	}{
		{"bare tcp address", ":8080", "tcp", ":8080"},
		{"host:port", "127.0.0.1:9090", "tcp", "127.0.0.1:9090"},
		{"absolute socket path", "unix:///run/rankedb.sock", "unix", "/run/rankedb.sock"},
		{"relative socket path", "unix://./rankedb.sock", "unix", "./rankedb.sock"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			network, address, err := parseAddr(tc.addr)
			if err != nil {
				t.Fatalf("parseAddr(%q): %v", tc.addr, err)
			}
			if network != tc.network || address != tc.address {
				t.Fatalf("parseAddr(%q) = (%q, %q), want (%q, %q)", tc.addr, network, address, tc.network, tc.address)
			}
		})
	}
	if _, _, err := parseAddr("unix://"); err == nil {
		t.Fatal(`parseAddr("unix://") = nil error, want one — no socket path given`)
	}
}

// TestNewRefusesSchemeWithNoPath pins the endpoint-level scenario: "unix://" alone
// fails New itself, not just parseAddr in isolation.
func TestNewRefusesSchemeWithNoPath(t *testing.T) {
	ctx := context.Background()
	cfg := scope.Literal(map[string]string{"addr": "unix://"})
	if _, err := New(ctx, cfg, core.New(nil, nil, nil, nil)); err == nil {
		t.Fatal(`New with "addr": "unix://" = nil error, want a build-time refusal`)
	}
}

// TestValidateAddr pins what the offline check catches: an overlong socket path, and a
// bare filesystem path missing the "unix://" scheme it needs to be read as one. A bare
// TCP address is unconstrained, exactly as before this change.
func TestValidateAddr(t *testing.T) {
	if err := ValidateAddr(":8080"); err != nil {
		t.Fatalf("ValidateAddr(bare tcp): %v", err)
	}
	if err := ValidateAddr("unix:///run/rankedb.sock"); err != nil {
		t.Fatalf("ValidateAddr(socket path): %v", err)
	}
	if err := ValidateAddr("/run/rankedb.sock"); err == nil {
		t.Fatal(`ValidateAddr("/run/rankedb.sock") = nil, want the "unix://" scheme correction`)
	}
	if err := ValidateAddr("unix://" + strings.Repeat("a", maxSocketPathLen+1)); err == nil {
		t.Fatal("ValidateAddr on an overlong socket path = nil, want an error")
	}
}

// builtServer builds a Server over the same no-op auth/access wiring newGrantedServer
// uses for TCP, for a case that needs the *Server itself rather than just its Handler.
func builtServer(t *testing.T, transport map[string]string) *Server {
	t.Helper()
	ctx := context.Background()

	a, err := auth.New(ctx, scope.Literal(map[string]string{"type": "noauth", "subject": "ops"}))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{"ops": everyReadRight})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	s, err := New(ctx, scope.Literal(transport), core.New(set, chk, nil, nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// unixTestServer builds a Server bound to a Unix domain socket path.
func unixTestServer(t *testing.T, sock string, transport map[string]string) *Server {
	t.Helper()
	cfg := map[string]string{"addr": "unix://" + sock}
	for k, v := range transport {
		cfg[k] = v
	}
	return builtServer(t, cfg)
}

// TestServeTCP pins that the TCP path still binds and serves for real after the switch
// to net.Listen + srv.Serve(ln) — the default path an address-form check must not move.
func TestServeTCP(t *testing.T) {
	reserved, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve a port: %v", err)
	}
	addr := reserved.Addr().String()
	_ = reserved.Close()

	s := builtServer(t, map[string]string{"addr": addr})
	sctx, cancel := context.WithCancel(context.Background())
	servec := make(chan error, 1)
	go func() { servec <- s.Serve(sctx) }()

	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err = http.Get("http://" + addr + "/health"); err == nil {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET /health over tcp: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	if err := <-servec; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

// TestServeUnixSocket pins that the endpoint can bind and serve a real request over a
// Unix domain socket, not just TCP, that its default mode admits only the server's own
// user, and that a clean shutdown unlinks the socket file behind it.
func TestServeUnixSocket(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "rankedb.sock")
	s := unixTestServer(t, sock, nil)

	sctx, cancel := context.WithCancel(context.Background())
	servec := make(chan error, 1)
	go func() { servec <- s.Serve(sctx) }()
	waitForSocket(t, sock)

	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("socket mode = %o, want 0600 (closed by default)", perm)
	}

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sock)
		},
	}}
	resp, err := client.Get("http://unix/health")
	if err != nil {
		t.Fatalf("GET /health over unix socket: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	cancel()
	if err := <-servec; err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket file survived a clean shutdown: %v", err)
	}
}

// TestServeUnixSocketMode pins that a declared "mode" is what the socket file
// actually carries, not just what was asked for.
func TestServeUnixSocketMode(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "rankedb.sock")
	s := unixTestServer(t, sock, map[string]string{"mode": "0660"})

	sctx, cancel := context.WithCancel(context.Background())
	servec := make(chan error, 1)
	go func() { servec <- s.Serve(sctx) }()
	waitForSocket(t, sock)
	defer func() {
		cancel()
		<-servec
	}()

	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o660 {
		t.Fatalf("socket mode = %o, want 0660", perm)
	}
}

// TestServeUnixSocketGroup pins that a declared "group" is the gid the socket file
// actually carries — the process's own primary group, resolved by name, not by gid.
func TestServeUnixSocketGroup(t *testing.T) {
	me, err := user.Current()
	if err != nil {
		t.Fatalf("user.Current: %v", err)
	}
	g, err := user.LookupGroupId(me.Gid)
	if err != nil {
		t.Fatalf("LookupGroupId(%s): %v", me.Gid, err)
	}

	sock := filepath.Join(t.TempDir(), "rankedb.sock")
	s := unixTestServer(t, sock, map[string]string{"mode": "0660", "group": g.Name})

	sctx, cancel := context.WithCancel(context.Background())
	servec := make(chan error, 1)
	go func() { servec <- s.Serve(sctx) }()
	waitForSocket(t, sock)
	defer func() {
		cancel()
		<-servec
	}()

	fi, err := os.Stat(sock)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatal("Sys() is not *syscall.Stat_t on this platform")
	}
	if gotGid := strconv.Itoa(int(stat.Gid)); gotGid != me.Gid {
		t.Fatalf("socket gid = %s, want %s (%s)", gotGid, me.Gid, g.Name)
	}
}

// TestUnknownGroupFailsAtLaunch pins that a "group" naming nobody on the host is
// reported clearly rather than left to fail obscurely at bind.
func TestUnknownGroupFailsAtLaunch(t *testing.T) {
	ctx := context.Background()
	a, err := auth.New(ctx, scope.Literal(map[string]string{"type": "noauth", "subject": "ops"}))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{"ops": everyReadRight})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	cfg := scope.Literal(map[string]string{"addr": "unix:///run/rankedb.sock", "group": "no-such-group-on-this-host"})
	if _, err := New(ctx, cfg, core.New(set, chk, nil, nil)); err == nil {
		t.Fatal("New with an unknown group = nil error, want one")
	}
}

// TestServeUnixSocketRemovesStaleFile pins that a leftover socket file from an
// unclean prior shutdown does not block a rebind — the situation an operator's
// process manager restarts into after a crash — while a live one refuses instead of
// being displaced, per design.md's "a stale socket is replaced; a live one is refused".
func TestServeUnixSocketRemovesStaleFile(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "rankedb.sock")

	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("stale socket file missing before the test even starts: %v", err)
	}

	s := unixTestServer(t, sock, nil)
	sctx, cancel := context.WithCancel(context.Background())
	servec := make(chan error, 1)
	go func() { servec <- s.Serve(sctx) }()
	waitForSocket(t, sock)

	cancel()
	if err := <-servec; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

// TestServeUnixSocketRefusesALiveOne pins the blocker case: a path where another
// instance is actually listening must refuse rather than unlink the running
// instance's socket out from under it.
func TestServeUnixSocketRefusesALiveOne(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "rankedb.sock")

	first := unixTestServer(t, sock, nil)
	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstServec := make(chan error, 1)
	go func() { firstServec <- first.Serve(firstCtx) }()
	waitForSocket(t, sock)
	defer func() {
		firstCancel()
		<-firstServec
	}()

	second := unixTestServer(t, sock, nil)
	if err := second.Serve(context.Background()); err == nil {
		t.Fatal("Serve against a live socket = nil error, want a refusal")
	}
	if _, err := os.Stat(sock); err != nil {
		t.Fatalf("the first instance's socket was removed: %v", err)
	}

	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return net.Dial("unix", sock)
		},
	}}
	resp, err := client.Get("http://unix/health")
	if err != nil {
		t.Fatalf("the first instance stopped answering after the refused second one: %v", err)
	}
	_ = resp.Body.Close()
}

// TestClaimSocketRefusesANonSocket pins the safety rail: a regular file sitting at
// the configured path is left alone rather than deleted out from under whatever put
// it there.
func TestClaimSocketRefusesANonSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-socket")
	if err := os.WriteFile(path, []byte("hi"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := claimSocket(path); err == nil {
		t.Fatal("claimSocket on a regular file = nil error, want one")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file was removed: %v", err)
	}
}

// waitForSocket polls until a socket file exists at path, or fails the test —
// binding happens in Serve's own goroutine, racing the test's own dial.
func waitForSocket(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("socket %s never appeared", path)
}

// foundArchive brings the archive into being, which construction no longer does: a
// Sequencer over an empty bookmark list reports InGenesis and refuses every other call
// until Found succeeds. The founding key is a throwaway — these cases exercise the
// endpoint, and nothing here contributes under it.
func foundArchive(t *testing.T, seq sequencer.Sequencer) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate founding key: %v", err)
	}
	encoded, err := ranke.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encode founding key: %v", err)
	}
	if _, err := seq.Found(context.Background(), encoded); err != nil {
		t.Fatalf("found the archive: %v", err)
	}
}
