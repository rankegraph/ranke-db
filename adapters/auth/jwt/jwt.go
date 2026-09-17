// package: jwt / authn
// type:    adapter
// job:     authenticate a bearer token by verifying it against a configured algorithm and key
// limits:  the key is static config or a refreshed JWKS (-> jwks.go); no network on the request path
//
// The token's own "alg" header is never trusted: verification is asked only for the
// algorithm config names, so "none" or an unlisted one is refused before the signature
// is even checked — a JWKS-sourced key does not relax this.
package jwt

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"github.com/rankegraph/ranke-db/adapters/auth/autherr"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// defaultAccountClaim is "sub" (RFC 7519's subject), unless config names a different
// claim — for an issuer whose "sub" is an opaque id rather than an account name.
const defaultAccountClaim = "sub"

// allowedAlgorithms is every signature algorithm this backend will ever accept; config
// must name one of these, never the token's own claim to use one.
var allowedAlgorithms = map[string]jose.SignatureAlgorithm{
	"HS256": jose.HS256, "HS384": jose.HS384, "HS512": jose.HS512,
	"RS256": jose.RS256, "RS384": jose.RS384, "RS512": jose.RS512,
	"ES256": jose.ES256, "ES384": jose.ES384, "ES512": jose.ES512,
	"PS256": jose.PS256, "PS384": jose.PS384, "PS512": jose.PS512,
	"EdDSA": jose.EdDSA,
}

// Auth is the JWT backend: an algorithm, a key source (static or JWKS), which claim
// names the account, and an audience/issuer to hold a token to (empty skips the check —
// an operator opts in by setting it).
type Auth struct {
	algorithm    jose.SignatureAlgorithm
	key          any         // set when configured with a static "key"
	jwks         *jwksSource // set when configured with "jwks_url" instead
	accountClaim string
	accounts     []accountRule // empty when the claim names the account
	audience     string
	issuer       string
}

// accountRule maps one value the claim may carry onto an account. Rules are read in
// config order, so precedence is the operator's, never the order an issuer listed in.
type accountRule struct{ value, account string }

// New builds the backend from "algorithm" (one of allowedAlgorithms) and exactly one
// key source: "key" — PEM public key preferred (a leaked config then cannot mint
// tokens, apikey's own standard for secrets), or a raw HMAC secret — or "jwks_url" for
// a rotating issuer, refreshed every "jwks_refresh" (default 5m). "account_claim"
// (default "sub"), "accounts", "audience" and "issuer" are optional.
func New(ctx context.Context, cfg scope.Section) (*Auth, error) {
	algName, err := cfg.Get(ctx, "algorithm")
	if err != nil {
		return nil, fmt.Errorf("jwt: algorithm: %w", err)
	}
	alg, ok := allowedAlgorithms[algName]
	if !ok {
		return nil, fmt.Errorf("jwt: algorithm %q is not in the allow-list", algName)
	}

	hasKey, hasJWKS := cfg.HasValue("key"), cfg.HasValue("jwks_url")
	switch {
	case hasKey && hasJWKS:
		return nil, errors.New("jwt: key and jwks_url are mutually exclusive")
	case !hasKey && !hasJWKS:
		return nil, errors.New("jwt: one of key or jwks_url is required")
	}

	var key any
	if hasKey {
		raw, err := cfg.Get(ctx, "key")
		if err != nil {
			return nil, fmt.Errorf("jwt: key: %w", err)
		}
		if key, err = parseKey(alg, raw); err != nil {
			return nil, fmt.Errorf("jwt: key: %w", err)
		}
	}

	a := &Auth{algorithm: alg, key: key, accountClaim: defaultAccountClaim}
	if cfg.HasValue("account_claim") {
		if a.accountClaim, err = cfg.Get(ctx, "account_claim"); err != nil {
			return nil, fmt.Errorf("jwt: account_claim: %w", err)
		}
		if a.accountClaim == "" {
			return nil, errors.New("jwt: account_claim must not be empty")
		}
	}
	if cfg.HasArray("accounts") {
		if a.accounts, err = accountRules(ctx, cfg); err != nil {
			return nil, err
		}
	}
	if cfg.HasValue("audience") {
		if a.audience, err = cfg.Get(ctx, "audience"); err != nil {
			return nil, fmt.Errorf("jwt: audience: %w", err)
		}
	}
	if cfg.HasValue("issuer") {
		if a.issuer, err = cfg.Get(ctx, "issuer"); err != nil {
			return nil, fmt.Errorf("jwt: issuer: %w", err)
		}
	}

	// Started last: every other field is already known good, so a JWKS refresh
	// goroutine never outlives a New call that fails after starting it.
	if hasJWKS {
		if a.jwks, err = newJWKSFromConfig(ctx, cfg); err != nil {
			return nil, fmt.Errorf("jwt: jwks_url: %w", err)
		}
	}
	return a, nil
}

// accountRules reads "accounts", an array of {"value": …, "account": …}.
func accountRules(ctx context.Context, cfg scope.Section) ([]accountRule, error) {
	entries := cfg.GetArray("accounts")
	rules := make([]accountRule, 0, len(entries))
	for i, entry := range entries {
		value, err := entry.Get(ctx, "value")
		if err != nil {
			return nil, fmt.Errorf("jwt: accounts[%d]: value: %w", i, err)
		}
		account, err := entry.Get(ctx, "account")
		if err != nil {
			return nil, fmt.Errorf("jwt: accounts[%d]: account: %w", i, err)
		}
		if value == "" || account == "" {
			return nil, fmt.Errorf("jwt: accounts[%d]: value and account are both required", i)
		}
		rules = append(rules, accountRule{value: value, account: account})
	}
	if len(rules) == 0 {
		return nil, errors.New("jwt: accounts is empty")
	}
	return rules, nil
}

// newJWKSFromConfig reads "jwks_url" (required) and "jwks_refresh" (optional, a
// time.ParseDuration string, default defaultJWKSRefresh) and starts a jwksSource.
func newJWKSFromConfig(ctx context.Context, cfg scope.Section) (*jwksSource, error) {
	url, err := cfg.Get(ctx, "jwks_url")
	if err != nil {
		return nil, err
	}
	interval := defaultJWKSRefresh
	if cfg.HasValue("jwks_refresh") {
		raw, err := cfg.Get(ctx, "jwks_refresh")
		if err != nil {
			return nil, err
		}
		if interval, err = time.ParseDuration(raw); err != nil {
			return nil, fmt.Errorf("jwks_refresh: %w", err)
		}
	}
	return newJWKSSource(url, interval)
}

// parseKey reads the raw config value as the shared secret an HS* algorithm verifies with, or
// as a PKIX PEM public key for every other algorithm.
func parseKey(alg jose.SignatureAlgorithm, raw string) (any, error) {
	if strings.HasPrefix(string(alg), "HS") {
		if raw == "" {
			return nil, errors.New("empty HMAC secret")
		}
		return []byte(raw), nil
	}
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, errors.New("not a PEM-encoded public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PKIX public key: %w", err)
	}
	return pub, nil
}

// Authenticate verifies token against the algorithm and key (by "kid", for a JWKS
// source — reading its cached set, never fetching), checks exp/nbf/iat and any
// configured audience/issuer, and resolves the account claim to Principal.Account. Any
// failure is ErrUnauthenticated — this backend states no opinion beyond "who".
func (a *Auth) Authenticate(_ context.Context, token string) (access.Principal, error) {
	parsed, err := jwt.ParseSigned(token, []jose.SignatureAlgorithm{a.algorithm})
	if err != nil {
		return access.Principal{}, autherr.ErrUnauthenticated
	}
	key := a.key
	if a.jwks != nil {
		if len(parsed.Headers) == 0 {
			return access.Principal{}, autherr.ErrUnauthenticated
		}
		found, ok := a.jwks.key(parsed.Headers[0].KeyID)
		if !ok {
			return access.Principal{}, autherr.ErrUnauthenticated
		}
		key = found
	}
	var registered jwt.Claims
	var raw map[string]json.RawMessage
	if err := parsed.Claims(key, &registered, &raw); err != nil {
		return access.Principal{}, autherr.ErrUnauthenticated
	}
	expected := jwt.Expected{Issuer: a.issuer}
	if a.audience != "" {
		expected.AnyAudience = jwt.Audience{a.audience}
	}
	if err := registered.Validate(expected); err != nil {
		return access.Principal{}, autherr.ErrUnauthenticated
	}
	account, err := a.account(raw)
	if err != nil {
		return access.Principal{}, autherr.ErrUnauthenticated
	}
	return access.Principal{Account: account}, nil
}

// account resolves the claim: it names the account itself, or carries values the rules
// map. Matching nothing authenticates nobody.
func (a *Auth) account(raw map[string]json.RawMessage) (string, error) {
	if len(a.accounts) == 0 {
		return stringClaim(raw, a.accountClaim)
	}
	held, err := claimValues(raw, a.accountClaim)
	if err != nil {
		return "", err
	}
	for _, rule := range a.accounts {
		if slices.Contains(held, rule.value) {
			return rule.account, nil
		}
	}
	return "", fmt.Errorf("claim %q carries no configured account", a.accountClaim)
}

// claimValues reads name as a string or as a list of them.
func claimValues(raw map[string]json.RawMessage, name string) ([]string, error) {
	msg, ok := raw[name]
	if !ok {
		return nil, fmt.Errorf("claim %q is absent", name)
	}
	var one string
	if err := json.Unmarshal(msg, &one); err == nil {
		return []string{one}, nil
	}
	var list []string
	if err := json.Unmarshal(msg, &list); err != nil {
		return nil, fmt.Errorf("claim %q is neither a string nor a list of them: %w", name, err)
	}
	return list, nil
}

// stringClaim reads name from the token's raw payload as a string — the account claim
// may be any registered or private claim, not just jwt.Claims's fixed fields.
func stringClaim(raw map[string]json.RawMessage, name string) (string, error) {
	msg, ok := raw[name]
	if !ok {
		return "", fmt.Errorf("claim %q is absent", name)
	}
	var s string
	if err := json.Unmarshal(msg, &s); err != nil {
		return "", fmt.Errorf("claim %q is not a string: %w", name, err)
	}
	if s == "" {
		return "", fmt.Errorf("claim %q is empty", name)
	}
	return s, nil
}

// Scheme is the credential scheme consumed — a literal equal to auth.SchemeBearer.
func (a *Auth) Scheme() string { return "bearer" }

// Close stops the background JWKS refresh loop; a no-op for a static key. Not part of
// the Auth port — a caller that knows it built a JWT backend may call it at shutdown.
func (a *Auth) Close() {
	if a.jwks != nil {
		a.jwks.close()
	}
}
