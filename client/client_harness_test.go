// package: client / transport
// type:    test-support
// job:     stand a real ranke-db behind the client — the rest_http endpoint over a core with an
// in-memory stack — so a case answers the server rather than a stub of the package's own making
// limits:  wiring only; what each case asserts is its own
//
// The endpoint is in this repository, so a contract drift fails here rather than passing
// against a mock that was written to agree.
package client_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/adapters/endpoints/rest_http"
	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// everyRight is what the test account holds unless a case narrows it: each reserved
// scope has to be granted by name, so the default names them all.
var everyRight = []string{"CR *", "C $branches", "R $universe", "R $archive", "R $branches"}

// testBranch is the branch the harness founds the archive on.
const testBranch = "main"

// testAccount is who a credential resolves to, whichever authenticator answers.
const testAccount = "ops"

// setup is what a case varies about the instance it is served.
type setup struct {
	grants  []string
	auth    scope.Section
	devClk  bool
	clients []client.Option
}

// option varies one thing about the instance a case is served.
type option func(*setup)

// withGrants replaces the account's grants, so a case can withhold one right and see
// what stops working.
func withGrants(grants ...string) option {
	return func(s *setup) { s.grants = grants }
}

// withAuth mounts an authenticator other than NoAuth, so a case can present a real
// credential and see which adapter it reaches.
func withAuth(cfg scope.Section) option {
	return func(s *setup) { s.auth = cfg }
}

// withoutDevClock leaves POST /dev/clock unmounted, which is every production stack.
func withoutDevClock() option {
	return func(s *setup) { s.devClk = false }
}

// as builds the client with the credential options given.
func as(opts ...client.Option) option {
	return func(s *setup) { s.clients = opts }
}

// stack is a running instance and the pieces a case reaches past the client for.
type stack struct {
	URL      string
	Universe ranke.Universe
	// Contributor signs the claims a case sends; the server's own identity attests the
	// merge, which is the separation the two keys exist for.
	Contributor ranke.Keypair
	Self        ranke.Claim
}

// serve stands up an instance and returns it with a client pointed at it.
func serve(t *testing.T, opts ...option) (*stack, *client.Client) {
	t.Helper()
	cfg := setup{
		grants: everyRight,
		auth:   scope.Literal(map[string]string{"type": "noauth", "subject": testAccount}),
		devClk: true,
	}
	for _, o := range opts {
		o(&cfg)
	}
	ctx := context.Background()

	sig, err := signer.New(ctx, scope.Literal(map[string]string{"type": "inmemory", "key": string(serverKey(t))}))
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}
	store := ranke.NewMemoryUniverse()
	// A steerable clock, so the archive's recorded times follow the story a case tells
	// rather than wall time — and so POST /dev/clock has something to steer.
	clock := sequencer.NewSteerableClock()
	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": t.Name()}), store, sig, clock.Now)
	if err != nil {
		t.Fatalf("sequencer.New: %v", err)
	}
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate the founding key: %v", err)
	}
	founding, err := ranke.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encode the founding key: %v", err)
	}
	if _, err := seq.Found(ctx, founding, testBranch); err != nil {
		t.Fatalf("found the archive: %v", err)
	}

	a, err := auth.New(ctx, cfg.auth)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{testAccount: cfg.grants})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	opt := []core.Option{core.WithSigner(sig)}
	if cfg.devClk {
		opt = append(opt, core.WithDevClock(clock.Advance))
	}
	srv, err := rest_http.New(ctx, scope.Literal(map[string]string{"addr": ":0"}),
		core.New(set, chk, seq, store, opt...))
	if err != nil {
		t.Fatalf("rest_http.New: %v", err)
	}
	listener := httptest.NewServer(srv.Handler())
	t.Cleanup(listener.Close)

	c, err := client.New(listener.URL, cfg.clients...)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	pair, self := contributor(t)
	return &stack{URL: listener.URL, Universe: store, Contributor: pair, Self: self}, c
}

// serverKey is the signing identity the instance attests its merges under, as the
// inmemory signer reads one.
func serverKey(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate the server key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal the server key: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// contributor mints the identity a case contributes under, with the contributor claim
// everything it signs resolves through.
func contributor(t *testing.T) (ranke.Keypair, ranke.Claim) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate a contributor key: %v", err)
	}
	pubkey, err := ranke.EncodePublicKey(priv.Public())
	if err != nil {
		t.Fatalf("encode a contributor key: %v", err)
	}
	self, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign the contributor claim: %v", err)
	}
	return ranke.Keypair{Private: priv, Pubkey: pubkey}, self
}

// note builds one signed claim citing the contributor, so a case has something to
// contribute that the closure can resolve.
func (s *stack) note(t *testing.T, text string, at time.Time) ranke.Claim {
	t.Helper()
	return s.claim(t, ranke.NewClaim("entity/note", s.asContributor(t)).
		WithInlineContent([]byte(text)).
		WithEncoding(ranke.EncodingText("plain")).
		WithCreatedAt(at).
		WithHeight(1))
}

// external builds a claim whose content lives in the Universe rather than inline, which
// is the case Contribute has to gather the bytes for.
func (s *stack) external(t *testing.T, content []byte, at time.Time) ranke.Claim {
	t.Helper()
	hash, err := ranke.HashContent(content)
	if err != nil {
		t.Fatalf("hash content: %v", err)
	}
	if err := s.Universe.PutContents(context.Background(),
		[]ranke.ContentBlob{{Hash: hash, Content: content}}); err != nil {
		t.Fatalf("put content: %v", err)
	}
	return s.claim(t, ranke.NewClaim("entity/document", s.asContributor(t)).
		WithExternalContent(hash, uint64(len(content))).
		WithEncoding(ranke.EncodingText("plain")).
		WithCreatedAt(at).
		WithHeight(1))
}

// asContributor resolves the contributor claim into the identity a builder signs under.
func (s *stack) asContributor(t *testing.T) ranke.Contributor {
	t.Helper()
	self, err := s.Self.AsContributor(context.Background(), nil, s.Contributor.Private)
	if err != nil {
		t.Fatalf("read the contributor claim: %v", err)
	}
	return self
}

// claim signs a built claim under the case's contributor key.
func (s *stack) claim(t *testing.T, b ranke.ClaimBuilder) ranke.Claim {
	t.Helper()
	c, err := b.Sign(s.Contributor.Private)
	if err != nil {
		t.Fatalf("sign a claim: %v", err)
	}
	return c
}

// seed contributes claims onto the test branch, steering the dev clock past them first
// so the merge is dated no earlier than what it absorbs.
func (s *stack) seed(t *testing.T, c *client.Client, claims ...ranke.Claim) *client.ContributionResult {
	t.Helper()
	ctx := context.Background()
	all := append([]ranke.Claim{s.Self}, claims...)
	if _, err := c.Dev().AdvanceClockPast(ctx, all); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	res, err := c.Contribute(ctx, s.Universe, testBranch, all)
	if err != nil {
		t.Fatalf("contribute: %v", err)
	}
	return res
}
