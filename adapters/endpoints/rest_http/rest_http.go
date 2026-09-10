// package: rest_http / transport
// type:    adapter
// job:     REST/HTTP endpoint backend — the Endpoints port and its server lifecycle
// limits:  transport + translation only; all capability lives behind core.Core (-> internal/core)
//
// Package rest_http implements the generated openapi.ServerInterface: it lifts the raw
// credential off the wire, builds a core.Request, and renders what core.Handle answers.
// Auth and access live in core; this is pure transport.
//
// By topic: auth.go (wire credential), respond.go (writers, error → status),
// endpoints_read.go, endpoints_contribute.go, endpoints_system.go,
// endpoints_verification.go — the Server's own lifecycle here.
package rest_http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core"
	"github.com/rankegraph/ranke-db/openapi"
)

// maxSocketPathLen is sun_path's usable length on Linux (108 bytes, minus the NUL).
const maxSocketPathLen = 107

// Server is a running REST/HTTP endpoint driving core.Handle.
type Server struct {
	core    *core.Core
	srv     *http.Server
	network string      // "tcp" or "unix"
	address string      // dial address, or a unix socket's filesystem path
	mode    os.FileMode // socket permission bits; meaningful only when network is "unix"
	group   *int        // socket gid, nil to leave the creating user's group
}

// New builds the endpoint from its config section; "addr" listens (default ":8080"), a
// "unix://" scheme naming a Unix domain socket instead of a TCP address.
func New(ctx context.Context, cfg scope.Section, c *core.Core) (*Server, error) {
	addr := ":8080"
	if cfg.HasValue("addr") {
		a, err := cfg.Get(ctx, "addr")
		if err != nil {
			return nil, fmt.Errorf("rest_http: addr: %w", err)
		}
		if a != "" {
			addr = a
		}
	}
	network, address, err := parseAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("rest_http: addr: %w", err)
	}

	mode := os.FileMode(0o600)
	var group *int
	if network == "unix" {
		var err error
		if mode, err = socketMode(ctx, cfg); err != nil {
			return nil, fmt.Errorf("rest_http: %w", err)
		}
		if group, err = socketGroup(ctx, cfg); err != nil {
			return nil, fmt.Errorf("rest_http: %w", err)
		}
	}

	cors, err := corsFrom(ctx, cfg)
	if err != nil {
		return nil, err
	}
	// "explorer" opts out of a build's embedded UI (default: served) — ignored, either
	// way, on a binary built without -tags explorer, which has nothing to serve regardless.
	explorer := true
	if cfg.HasValue("explorer") {
		v, err := cfg.Get(ctx, "explorer")
		if err != nil {
			return nil, fmt.Errorf("rest_http: explorer: %w", err)
		}
		explorer = v != "false"
	}

	s := &Server{core: c, network: network, address: address, mode: mode, group: group}
	// Both sit outside the generated router: a static asset and a build banner, neither
	// part of the OpenAPI contract, while the generated handler owns "/" as its catch-all.
	mux := http.NewServeMux()
	mux.Handle("/explorer", explorerHandler(explorer))
	mux.Handle("/{$}", rootHandler())
	mux.Handle("/", openapi.Handler(s))
	// CORS outermost: a preflight carries no credential and must be answered before the
	// credential is extracted, since a browser sends it without one.
	handler := cors.withCORS(s.withCredential(mux))
	s.srv = &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	return s, nil
}

// parseAddr decides the network from addr's scheme: "unix://path" names a Unix domain
// socket at path, and every other value is a TCP address, as it always was.
func parseAddr(addr string) (network, address string, err error) {
	if rest, ok := strings.CutPrefix(addr, "unix://"); ok {
		if rest == "" {
			return "", "", errors.New(`"unix://" names no socket path`)
		}
		return "unix", rest, nil
	}
	return "tcp", addr, nil
}

// ValidateAddr checks addr's form offline: a socket path must fit the sun_path limit,
// and a bare filesystem path missing the scheme is reported with the correction.
func ValidateAddr(addr string) error {
	if rest, ok := strings.CutPrefix(addr, "unix://"); ok {
		if len(rest) > maxSocketPathLen {
			return fmt.Errorf("socket path %q is %d bytes, over the %d-byte limit a Unix domain socket allows", rest, len(rest), maxSocketPathLen)
		}
		return nil
	}
	if strings.HasPrefix(addr, "/") || strings.HasPrefix(addr, "./") {
		return fmt.Errorf(`address %q needs the "unix://" scheme to be read as a socket path`, addr)
	}
	return nil
}

// socketMode reads "mode" as octal permission bits, defaulting to 0600 — the server's
// own user and nobody else, the same closed-by-default posture as allowedOrigins.
func socketMode(ctx context.Context, cfg scope.Section) (os.FileMode, error) {
	if !cfg.HasValue("mode") {
		return 0o600, nil
	}
	raw, err := cfg.Get(ctx, "mode")
	if err != nil {
		return 0, fmt.Errorf("mode: %w", err)
	}
	bits, err := strconv.ParseUint(strings.TrimPrefix(raw, "0o"), 8, 32)
	if err != nil || bits > 0o777 {
		return 0, fmt.Errorf("mode %q is not octal permission bits", raw)
	}
	return os.FileMode(bits), nil
}

// socketGroup reads the optional "group" — a name or a gid — resolving a name now so
// a typo fails at launch rather than leaving the socket owned by no one that connects.
func socketGroup(ctx context.Context, cfg scope.Section) (*int, error) {
	if !cfg.HasValue("group") {
		return nil, nil
	}
	raw, err := cfg.Get(ctx, "group")
	if err != nil {
		return nil, fmt.Errorf("group: %w", err)
	}
	if gid, err := strconv.Atoi(raw); err == nil {
		return &gid, nil
	}
	g, err := user.LookupGroup(raw)
	if err != nil {
		return nil, fmt.Errorf("group %q: %w", raw, err)
	}
	gid, err := strconv.Atoi(g.Gid)
	if err != nil {
		return nil, fmt.Errorf("group %q: unexpected gid %q", raw, g.Gid)
	}
	return &gid, nil
}

// Handler is the routed endpoint, credential extraction and CORS included, without a
// listener under it. It is what Serve binds, so a caller driving the routes directly —
// the Go client's tests, which answer the real handler rather than a stub of their own
// making — exercises the same stack a request over a socket reaches.
func (s *Server) Handler() http.Handler { return s.srv.Handler }

// Serve binds the endpoint's listener and runs until ctx is cancelled, then shuts
// down gracefully. Binding happens here, not in New, so building never claims a port.
func (s *Server) Serve(ctx context.Context) error {
	if s.network == "unix" {
		if err := claimSocket(s.address); err != nil {
			return fmt.Errorf("rest_http: %w", err)
		}
	}
	ln, err := net.Listen(s.network, s.address)
	if err != nil {
		return fmt.Errorf("rest_http: listen: %w", err)
	}
	if s.network == "unix" {
		// Applied before Serve, so nothing is answered under the umask's wider bits.
		if err := os.Chmod(s.address, s.mode); err != nil {
			return fmt.Errorf("rest_http: chmod socket: %w", err)
		}
		if s.group != nil {
			if err := os.Chown(s.address, -1, *s.group); err != nil {
				return fmt.Errorf("rest_http: chown socket: %w", err)
			}
		}
	}

	errc := make(chan error, 1)
	go func() { errc <- s.srv.Serve(ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		return s.shutdown()
	}
}

// claimSocket refuses a path where another instance is listening, replaces one
// nobody is (residue from an unclean shutdown), and leaves anything else alone.
func claimSocket(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat %s: %w", path, err)
	}
	if fi.Mode().Type() != os.ModeSocket {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	conn, err := net.DialTimeout("unix", path, 200*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("%s: another instance is already listening", path)
	}
	return os.Remove(path)
}

// Close shuts the endpoint down.
func (s *Server) Close() error { return s.shutdown() }

func (s *Server) shutdown() error {
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.srv.Shutdown(sctx)
}
