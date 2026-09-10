package macaroon

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	joemacaroon "gopkg.in/macaroon.v2"

	"github.com/rankegraph/ranke-db/adapters/auth/autherr"
	"github.com/rankegraph/ranke-db/adapters/auth/macaroon/macaroontest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// newAuth builds the backend under test against macaroontest.RootKey — every case
// below mints with that key and asks whether Authenticate accepts or rejects it.
func newAuth(t *testing.T) *Auth {
	t.Helper()
	a, err := New(context.Background(), scope.Literal(map[string]string{
		"type": "macaroon", "root_key": macaroontest.RootKey,
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestValidMacaroonResolvesToItsAccount(t *testing.T) {
	a := newAuth(t)
	token := macaroontest.Mint(t, macaroontest.RootKey, "webapp")

	p, err := a.Authenticate(context.Background(), token)
	if err != nil || p.Account != "webapp" || len(p.Caveats) != 0 {
		t.Fatalf("account=%q caveats=%v err=%v, want webapp/none/nil", p.Account, p.Caveats, err)
	}
}

// TestCaveatNarrowsToOneBranch: a first-party caveat carrying grant syntax is
// parsed into Principal.Caveats exactly as access.ParseGrant would build it — the
// enforcement itself is access.Allow's job (already tested), this only checks the
// translation lands.
func TestCaveatNarrowsToOneBranch(t *testing.T) {
	a := newAuth(t)
	token := macaroontest.Mint(t, macaroontest.RootKey, "webapp", "R foo_bar")

	p, err := a.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if len(p.Caveats) != 1 || !p.Caveats[0].Allows('R', "foo_bar") || p.Caveats[0].Allows('C', "foo_bar") {
		t.Fatalf("caveats=%v, want exactly one Grant('R foo_bar')", p.Caveats)
	}
}

// TestAttemptedWideningIsRefused: an attacker without the root key tampers with a
// narrowly-caveated macaroon's serialized caveat condition to claim broader
// rights. The HMAC chain was computed over the ORIGINAL bytes, so this must fail
// signature verification — the property that makes attenuation safe in the first
// place, since adding a caveat needs no root key but changing an EXISTING one
// still has to fool the signature.
func TestAttemptedWideningIsRefused(t *testing.T) {
	a := newAuth(t)
	token := macaroontest.Mint(t, macaroontest.RootKey, "webapp", "R foo_bar")
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	widened := bytes.Replace(raw, []byte("R foo_bar"), []byte("D foo_bar"), 1)
	if bytes.Equal(raw, widened) {
		t.Fatal("fixture did not contain the caveat condition verbatim — test is not exercising anything")
	}
	tampered := base64.RawURLEncoding.EncodeToString(widened)

	if _, err := a.Authenticate(context.Background(), tampered); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("widened caveat: err=%v, want ErrUnauthenticated", err)
	}
}

func TestTamperedSignatureIsRefused(t *testing.T) {
	a := newAuth(t)
	token := macaroontest.Mint(t, macaroontest.RootKey, "webapp")
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decoding fixture: %v", err)
	}
	raw[len(raw)-1] ^= 0xff // the signature sits at the end of a V2 macaroon's packet
	tampered := base64.RawURLEncoding.EncodeToString(raw)

	if _, err := a.Authenticate(context.Background(), tampered); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("tampered signature: err=%v, want ErrUnauthenticated", err)
	}
}

// TestUnknownSyntaxCaveatFailsClosed: a caveat this backend cannot parse must
// refuse the whole macaroon, not silently drop the caveat — dropping it would
// hand back a WIDER token (no caveat at all) than the one actually presented.
func TestUnknownSyntaxCaveatFailsClosed(t *testing.T) {
	a := newAuth(t)
	token := macaroontest.Mint(t, macaroontest.RootKey, "webapp", "not a grant spec")

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("unparseable caveat: err=%v, want ErrUnauthenticated", err)
	}
}

// TestThirdPartyCaveatIsRefused: no discharge protocol exists here, so a
// macaroon requiring one can never verify — refused, not silently ignored.
func TestThirdPartyCaveatIsRefused(t *testing.T) {
	m, err := joemacaroon.New([]byte(macaroontest.RootKey), []byte("webapp"), "", joemacaroon.V2)
	if err != nil {
		t.Fatalf("macaroon.New: %v", err)
	}
	if err := m.AddThirdPartyCaveat([]byte("third-party-root-key"), []byte("caveat-id"), "https://elsewhere.example"); err != nil {
		t.Fatalf("AddThirdPartyCaveat: %v", err)
	}
	raw, err := m.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	token := base64.RawURLEncoding.EncodeToString(raw)

	a := newAuth(t)
	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("third-party caveat, no discharge: err=%v, want ErrUnauthenticated", err)
	}
}

func TestNewRejects(t *testing.T) {
	ctx := context.Background()
	bad := map[string]scope.Section{
		"missing root_key": scope.Literal(map[string]string{}),
		"empty root_key":   scope.Literal(map[string]string{"root_key": ""}),
	}
	for name, cfg := range bad {
		if _, err := New(ctx, cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestScheme(t *testing.T) {
	if got := (&Auth{}).Scheme(); got != "macaroon" {
		t.Fatalf("Scheme() = %q, want macaroon", got)
	}
}
