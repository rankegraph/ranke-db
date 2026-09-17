package jwt

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	josejwt "github.com/go-jose/go-jose/v4/jwt"

	"github.com/rankegraph/ranke-db/adapters/auth/autherr"
	"github.com/rankegraph/ranke-db/adapters/auth/jwt/jwttest"
	"github.com/rankegraph/ranke-db/config/scope"
)

// rsaKey mints a throwaway RSA key for the algorithm-confusion fixture — the point
// is a real, differently-algorithmed signature, not this key's own strength.
func rsaKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	return key
}

// signWithExtra signs an arbitrary claim set — a plain map rather than jwt.Claims —
// for fixtures needing a private claim (like "email") jwt.Claims has no field for.
func signWithExtra(t *testing.T, priv ed25519.PrivateKey, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: priv}, nil)
	if err != nil {
		t.Fatalf("jose.NewSigner: %v", err)
	}
	token, err := josejwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}
	return token
}

// corruptSignature flips a byte in a compact JWT's signature segment, leaving its
// header and payload — and so its claims — untouched.
func corruptSignature(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decoding signature: %v", err)
	}
	if len(sig) == 0 {
		t.Fatal("empty signature — nothing to corrupt")
	}
	sig[0] ^= 0xff
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	return strings.Join(parts, ".")
}

// newAuth builds the backend under test directly against pub, the EdDSA algorithm's
// only accepted form here — every case below signs with jwttest and asks whether
// Authenticate accepts or rejects it, never asserting on how.
func newAuth(t *testing.T, pub ed25519.PublicKey) *Auth {
	t.Helper()
	return newAuthWith(t, pub, nil)
}

// newAuthWith is newAuth plus extra config fields (account_claim, audience, issuer),
// for the cases exercising those specifically.
func newAuthWith(t *testing.T, pub ed25519.PublicKey, extra map[string]string) *Auth {
	t.Helper()
	values := map[string]string{"type": "jwt", "algorithm": "EdDSA", "key": jwttest.PublicKeyPEM(t, pub)}
	for k, v := range extra {
		values[k] = v
	}
	a, err := New(context.Background(), scope.Literal(values))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestAlgNoneIsRejected covers the classic JWT vulnerability directly: a token that
// declares no signature at all must never authenticate, however permissive a naive
// verifier might be. Hand-built, since jwttest's Sign always signs with a real key —
// there is no way to reach an "alg: none" token through it.
func TestAlgNoneIsRejected(t *testing.T) {
	_, priv := jwttest.GenerateKey(t)
	a := newAuth(t, priv.Public().(ed25519.PublicKey))

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"attacker"}`))
	token := header + "." + payload + "."

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("alg none: err=%v, want ErrUnauthenticated", err)
	}
}

// TestDisallowedAlgorithmIsRejected covers algorithm confusion: a token signed with
// an algorithm the config does not name must be refused even though its own "alg"
// header is a real one and its signature genuinely verifies under that algorithm —
// the point of the allow-list is that the token's own header is never trusted.
func TestDisallowedAlgorithmIsRejected(t *testing.T) {
	_, priv := jwttest.GenerateKey(t)
	rsaPriv := rsaKey(t)
	a := newAuth(t, priv.Public().(ed25519.PublicKey)) // configured for EdDSA only

	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: rsaPriv}, nil)
	if err != nil {
		t.Fatalf("jose.NewSigner: %v", err)
	}
	token, err := josejwt.Signed(signer).Claims(josejwt.Claims{Subject: "attacker"}).Serialize()
	if err != nil {
		t.Fatalf("Serialize: %v", err)
	}

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("RS256 token against an EdDSA-only config: err=%v, want ErrUnauthenticated", err)
	}
}

// TestExpiredTokenIsRejected: exp is not optional. A backend that ignored it would
// authenticate a token forever.
func TestExpiredTokenIsRejected(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuth(t, pub)
	token := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "conformance-account",
		Expiry:  josejwt.NewNumericDate(time.Now().Add(-time.Hour)),
	})

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("expired token: err=%v, want ErrUnauthenticated", err)
	}
}

// TestWrongSignatureIsRejected: a byte flipped in the signature segment must not
// verify — the base case any signature scheme is built to guarantee.
func TestWrongSignatureIsRejected(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuth(t, pub)
	token := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "conformance-account",
		Expiry:  josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	corrupted := corruptSignature(t, token)

	if _, err := a.Authenticate(context.Background(), corrupted); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("corrupted signature: err=%v, want ErrUnauthenticated", err)
	}
}

// TestTokenFromADifferentKeyIsRejected: signed correctly, with the right algorithm,
// by a key this backend was never configured to trust.
func TestTokenFromADifferentKeyIsRejected(t *testing.T) {
	pub, _ := jwttest.GenerateKey(t)
	_, otherPriv := jwttest.GenerateKey(t)
	a := newAuth(t, pub)
	token := jwttest.Sign(t, otherPriv, josejwt.Claims{
		Subject: "conformance-account",
		Expiry:  josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("token from a different key: err=%v, want ErrUnauthenticated", err)
	}
}

// TestAccountClaimIsConfigurable: an operator whose issuer's "sub" is an opaque
// internal id, not something matching a configured account name, can name a
// different claim to resolve the account from instead — and the default is still
// "sub" when account_claim is not set.
func TestAccountClaimIsConfigurable(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWith(t, pub, map[string]string{"account_claim": "email"})
	token := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "opaque-internal-id",
		Expiry:  josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	// "email" is not a registered claim jwt.Claims carries, so it has to be added
	// to the token by hand rather than through jwttest.Sign's Claims struct.
	tokenWithEmail := signWithExtra(t, priv, map[string]any{
		"sub": "opaque-internal-id", "email": "person@example.com",
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	if _, err := a.Authenticate(context.Background(), token); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("token without the configured claim: err=%v, want ErrUnauthenticated", err)
	}
	p, err := a.Authenticate(context.Background(), tokenWithEmail)
	if err != nil || p.Account != "person@example.com" {
		t.Fatalf("account=%q err=%v, want person@example.com/nil", p.Account, err)
	}
}

// TestAudienceIssuerUncheckedWhenNotConfigured: an operator who never set audience
// or issuer accepts a token naming either — those checks are opt-in, not a default
// every deployment pays for.
func TestAudienceIssuerUncheckedWhenNotConfigured(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuth(t, pub)
	token := jwttest.Sign(t, priv, josejwt.Claims{
		Subject:  "conformance-account",
		Audience: josejwt.Audience{"some-other-service"},
		Issuer:   "some-other-issuer",
		Expiry:   josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})

	if _, err := a.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("unconfigured audience/issuer: err=%v, want nil", err)
	}
}

// TestAudienceIssuerEnforcedWhenConfigured: once an operator sets audience and/or
// issuer, a token for a different one is refused — the check the task's own
// reasoning asked for ('a token valid for another service should not authenticate
// here'), applied only where an operator opted in.
func TestAudienceIssuerEnforcedWhenConfigured(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWith(t, pub, map[string]string{"audience": "ranke-db", "issuer": "trusted-issuer"})

	right := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "conformance-account", Audience: josejwt.Audience{"ranke-db"}, Issuer: "trusted-issuer",
		Expiry: josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	if _, err := a.Authenticate(context.Background(), right); err != nil {
		t.Fatalf("matching audience/issuer: err=%v, want nil", err)
	}

	wrongAudience := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "conformance-account", Audience: josejwt.Audience{"some-other-service"}, Issuer: "trusted-issuer",
		Expiry: josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	if _, err := a.Authenticate(context.Background(), wrongAudience); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("wrong audience: err=%v, want ErrUnauthenticated", err)
	}

	wrongIssuer := jwttest.Sign(t, priv, josejwt.Claims{
		Subject: "conformance-account", Audience: josejwt.Audience{"ranke-db"}, Issuer: "some-other-issuer",
		Expiry: josejwt.NewNumericDate(time.Now().Add(time.Hour)),
	})
	if _, err := a.Authenticate(context.Background(), wrongIssuer); !errors.Is(err, autherr.ErrUnauthenticated) {
		t.Fatalf("wrong issuer: err=%v, want ErrUnauthenticated", err)
	}
}

// TestNewRejects covers the configs New must reject: an empty or unknown algorithm,
// a missing key, a key that is not valid PEM, and an asymmetric algorithm's key that
// does not parse as a PKIX public key at all.
func TestNewRejects(t *testing.T) {
	pub, _ := jwttest.GenerateKey(t)
	ctx := context.Background()
	bad := map[string]scope.Section{
		"empty algorithm":   scope.Literal(map[string]string{"key": jwttest.PublicKeyPEM(t, pub)}),
		"unknown algorithm": scope.Literal(map[string]string{"algorithm": "HS1", "key": "secret"}),
		"missing key":       scope.Literal(map[string]string{"algorithm": "EdDSA"}),
		"empty HMAC secret": scope.Literal(map[string]string{"algorithm": "HS256", "key": ""}),
		"non-PEM key":       scope.Literal(map[string]string{"algorithm": "EdDSA", "key": "not-a-pem"}),
	}
	for name, cfg := range bad {
		if _, err := New(ctx, cfg); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestScheme(t *testing.T) {
	if got := (&Auth{}).Scheme(); got != "bearer" {
		t.Fatalf("Scheme() = %q, want bearer", got)
	}
}

// newAuthWithRules builds the backend with an ordered value-to-account mapping over
// claim, the shape a directory's roles arrive in.
func newAuthWithRules(t *testing.T, pub ed25519.PublicKey, claim string, rules ...[2]string) *Auth {
	t.Helper()
	entries := make([]scope.Section, 0, len(rules))
	for _, r := range rules {
		entries = append(entries, scope.Literal(map[string]string{"value": r[0], "account": r[1]}))
	}
	a, err := New(context.Background(), scope.LiteralArray(
		map[string]string{
			"type": "jwt", "algorithm": "EdDSA",
			"key": jwttest.PublicKeyPEM(t, pub), "account_claim": claim,
		},
		map[string][]scope.Section{"accounts": entries},
	))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

// TestRoleClaimResolvesToAnAccount: an issuer that states roles rather than accounts —
// Entra's app roles, a directory's groups — is read through the mapping, so several
// people hold one service account and adding a person is an assignment there, not an
// edit here.
func TestRoleClaimResolvesToAnAccount(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWithRules(t, pub, "roles", [2]string{"ranke-admin", "admin"}, [2]string{"ranke-ops", "ops"})

	token := signWithExtra(t, priv, map[string]any{
		"sub": "1ff1de77-4005-4d0b-b3f2-1cfbf82d6b23", "roles": []string{"ranke-ops"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	principal, err := a.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if principal.Account != "ops" {
		t.Fatalf("Account = %q, want %q", principal.Account, "ops")
	}
}

// TestConfiguredOrderDecidesTheAccount: a subject holding both roles acts as the account
// configured first, whatever order the issuer listed them in — precedence an operator
// reads off the config, rather than one an issuer could change under them.
func TestConfiguredOrderDecidesTheAccount(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWithRules(t, pub, "roles", [2]string{"ranke-admin", "admin"}, [2]string{"ranke-ops", "ops"})

	token := signWithExtra(t, priv, map[string]any{
		"sub": "abc", "roles": []string{"ranke-ops", "ranke-admin"},
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	principal, err := a.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if principal.Account != "admin" {
		t.Fatalf("Account = %q, want %q", principal.Account, "admin")
	}
}

// TestRoleOutsideTheMappingIsRejected: a valid token whose roles name no account
// authenticates nobody. A directory full of roles this server never heard of must not
// yield a principal, and an unassigned person is refused rather than admitted bare.
func TestRoleOutsideTheMappingIsRejected(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWithRules(t, pub, "roles", [2]string{"ranke-admin", "admin"})

	for name, claims := range map[string]map[string]any{
		"another app's role": {"roles": []string{"some-other-app-role"}},
		"no roles at all":    {"sub": "abc"},
		"an empty list":      {"roles": []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			claims["exp"] = time.Now().Add(time.Hour).Unix()
			_, err := a.Authenticate(context.Background(), signWithExtra(t, priv, claims))
			if !errors.Is(err, autherr.ErrUnauthenticated) {
				t.Fatalf("Authenticate: %v, want ErrUnauthenticated", err)
			}
		})
	}
}

// TestASingleValuedClaimStillMaps: the mapping reads a plain string claim too, so an
// application identity — whose azp is one value, not a list — maps the same way.
func TestASingleValuedClaimStillMaps(t *testing.T) {
	pub, priv := jwttest.GenerateKey(t)
	a := newAuthWithRules(t, pub, "azp", [2]string{"6e74172b-be56-4843-9ff4-e66a39bb12e3", "explorer"})

	token := signWithExtra(t, priv, map[string]any{
		"azp": "6e74172b-be56-4843-9ff4-e66a39bb12e3",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	principal, err := a.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if principal.Account != "explorer" {
		t.Fatalf("Account = %q, want %q", principal.Account, "explorer")
	}
}
