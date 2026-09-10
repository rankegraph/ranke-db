// package: client / transport
// type:    test
// job:     construction and credentials — what a base URL may be, that one credential is presented
// and several refused, and which authenticator each scheme reaches
// limits:  the client's own decisions; what a route answers is read_test.go's
package client_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rankegraph/ranke-db/adapters/auth/apikey/apikeytest"
	"github.com/rankegraph/ranke-db/adapters/auth/macaroon/macaroontest"
	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/config/scope"
)

// closedAddr is an address nothing is listening on: a port bound long enough to be
// allocated, then released.
func closedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return addr
}

// TestNewDefaultsABareHostToHTTP pins the gap the generated constructor leaves: it
// appends a trailing slash to the server string and nothing else, so a bare host:port
// would build a request no transport can dial.
func TestNewDefaultsABareHostToHTTP(t *testing.T) {
	for _, tc := range []struct{ given, want string }{
		{"localhost:8080", "http://localhost:8080"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"https://ranke.example/", "https://ranke.example"},
		{"example.internal", "http://example.internal"},
		{"  localhost:9999  ", "http://localhost:9999"},
	} {
		t.Run(tc.given, func(t *testing.T) {
			c, err := client.New(tc.given)
			if err != nil {
				t.Fatalf("New(%q): %v", tc.given, err)
			}
			if got := c.BaseURL(); got != tc.want {
				t.Fatalf("BaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestNewRefusesNoBaseURL pins that a client against nothing is refused where it is
// built, not on the first request.
func TestNewRefusesNoBaseURL(t *testing.T) {
	for _, given := range []string{"", "   ", "http://"} {
		if _, err := client.New(given); !errors.Is(err, client.ErrNoBaseURL) {
			t.Fatalf("New(%q) = %v, want ErrNoBaseURL", given, err)
		}
	}
}

// TestNewRefusesSeveralCredentials pins exclusivity at construction. The endpoint reads
// both the Authorization and the X-API-Key header and answers 400 ambiguous when both
// are there, so two credentials name two authenticators and the request has no single
// answer — which is worth saying before a socket is opened.
func TestNewRefusesSeveralCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []client.Option
	}{
		{"token and api key", []client.Option{client.WithToken("t"), client.WithAPIKey("k0123456789abcdef")}},
		{"token and macaroon", []client.Option{client.WithToken("t"), client.WithMacaroon("m")}},
		{"api key and macaroon", []client.Option{client.WithAPIKey("k0123456789abcdef"), client.WithMacaroon("m")}},
		{"all three", []client.Option{client.WithToken("t"), client.WithAPIKey("k0123456789abcdef"), client.WithMacaroon("m")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.New("localhost:8080", tc.opts...)
			if !errors.Is(err, client.ErrCredentials) {
				t.Fatalf("New = %v, want ErrCredentials", err)
			}
			if !strings.Contains(err.Error(), " and ") {
				t.Fatalf("New = %q, want the credentials it saw named", err)
			}
		})
	}
}

// TestNoCredentialReachesNoAuth pins that presenting nothing is a mode of its own:
// noauth registers under the empty scheme and is reachable only through the fallback,
// so a request carrying any credential header never arrives there.
func TestNoCredentialReachesNoAuth(t *testing.T) {
	_, c := serve(t)
	who, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if who.Account != testAccount {
		t.Fatalf("account = %q, want %q", who.Account, testAccount)
	}
}

// TestAPIKeyReachesTheAPIKeyAuthenticator pins that WithAPIKey presents X-API-Key,
// which is the header the endpoint routes to the apikey backend.
func TestAPIKeyReachesTheAPIKeyAuthenticator(t *testing.T) {
	cfg, key, done := apikeytest.Setup(t, testAccount)
	t.Cleanup(done)
	_, c := serve(t, withAuth(cfg), as(client.WithAPIKey(key)))

	who, err := c.Whoami(context.Background())
	if err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if who.Account != testAccount {
		t.Fatalf("account = %q, want %q", who.Account, testAccount)
	}
}

// TestMacaroonIsItsOwnCredential is why WithMacaroon is not optional. The endpoint
// lowercases the first token of Authorization and routes "macaroon" to the macaroon
// adapter, everything else — Bearer included — to the JWT one. The same macaroon
// through WithToken therefore reaches an authenticator that cannot read it, and 401s
// with nothing naming the cause.
func TestMacaroonIsItsOwnCredential(t *testing.T) {
	cfg := scope.Literal(map[string]string{"type": "macaroon", "root_key": macaroontest.RootKey})
	token := macaroontest.Mint(t, macaroontest.RootKey, testAccount, "R "+testBranch)

	t.Run("as a macaroon", func(t *testing.T) {
		_, c := serve(t, withAuth(cfg), as(client.WithMacaroon(token)))
		who, err := c.Whoami(context.Background())
		if err != nil {
			t.Fatalf("whoami: %v", err)
		}
		if who.Account != testAccount {
			t.Fatalf("account = %q, want %q", who.Account, testAccount)
		}
		// The caveats are the runtime attenuation only a macaroon carries, and the
		// reason it cannot be folded into WithToken.
		if len(who.Caveats) != 1 || who.Caveats[0] != "R "+testBranch {
			t.Fatalf("caveats = %v, want the one first-party caveat it was minted with", who.Caveats)
		}
	})

	t.Run("as a bearer token", func(t *testing.T) {
		_, c := serve(t, withAuth(cfg), as(client.WithToken(token)))
		_, err := c.Whoami(context.Background())
		if !errors.Is(err, client.ErrUnauthenticated) {
			t.Fatalf("whoami = %v, want ErrUnauthenticated — Bearer routes to the JWT adapter", err)
		}
	})
}

// TestWaitReadyStopsAtARefusal pins the distinction WaitReady exists to make: nothing
// listening is worth waiting out, a listener that answers and refuses is not, and
// retrying the second only spends the whole window on an answer already given.
func TestWaitReadyStopsAtARefusal(t *testing.T) {
	t.Run("a listening instance", func(t *testing.T) {
		_, c := serve(t)
		if err := c.WaitReady(context.Background(), time.Second); err != nil {
			t.Fatalf("WaitReady: %v", err)
		}
	})

	t.Run("listening and refusing", func(t *testing.T) {
		cfg, _, done := apikeytest.Setup(t, testAccount)
		t.Cleanup(done)
		_, c := serve(t, withAuth(cfg), as(client.WithAPIKey("not-the-configured-key")))
		start := time.Now()
		err := c.WaitReady(context.Background(), 5*time.Second)
		if !errors.Is(err, client.ErrUnauthenticated) {
			t.Fatalf("WaitReady = %v, want the refusal itself", err)
		}
		if waited := time.Since(start); waited > time.Second {
			t.Fatalf("WaitReady spent %s retrying an answer already given", waited)
		}
	})

	t.Run("a refusal waiting can change", func(t *testing.T) {
		// What a proxy answers for a backend still coming up. Giving up on it would
		// defeat the one thing this method is for.
		c := refusing(t, http.StatusServiceUnavailable, "backend not ready")
		start := time.Now()
		if err := c.WaitReady(context.Background(), 200*time.Millisecond); err == nil {
			t.Fatal("WaitReady against a 503 returned nil")
		}
		if waited := time.Since(start); waited < 100*time.Millisecond {
			t.Fatalf("WaitReady gave up after %s, want it to wait the 503 out", waited)
		}
	})

	t.Run("nothing listening", func(t *testing.T) {
		c, err := client.New(closedAddr(t), client.WithHTTPClient(&http.Client{Timeout: time.Second}))
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		start := time.Now()
		if err := c.WaitReady(context.Background(), 200*time.Millisecond); err == nil {
			t.Fatal("WaitReady against a closed port returned nil")
		}
		if waited := time.Since(start); waited < 100*time.Millisecond {
			t.Fatalf("WaitReady gave up after %s, want it to wait out the window", waited)
		}
	})
}
