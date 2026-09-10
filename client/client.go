// package: client / transport
// type:    adapter
// job:     the official Go client for a running ranke-db — one base URL, one credential, and the
// generated request plumbing every call shares
// limits:  transport only; what a route means is the OpenAPI contract's (-> openapi/openapi.yaml)
// and what travels over it is the library's (-> ranke-go)
//
// Every request and path string comes from openapi/client, generated from the contract,
// so a spec change breaks this build where a hand-written path would drift in silence.
//
// By topic: errors.go, read.go, branches.go, system.go, verification.go, contribute.go,
// dev.go, seq.go (result framing).
package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	api "github.com/rankegraph/ranke-db/openapi/client"
)

// Sentinels for what a caller gets wrong before a request is made; a refusal from the
// server arrives as an *Error.
var (
	// ErrNoBaseURL is a client built against nothing.
	ErrNoBaseURL = errors.New("ranke/client: base URL is required")
	// ErrCredentials is several credentials at once, which the endpoint cannot route.
	ErrCredentials = errors.New("ranke/client: present one credential, not several")
	// ErrNilClaim is a nil claim, or a nil id in a by-id read.
	ErrNilClaim = errors.New("ranke/client: nil claim")
	// ErrNoPubkey is a read narrowed to a key, with no key given.
	ErrNoPubkey = errors.New("ranke/client: a pubkey is required")
	// ErrUniverseNeedsHead is a closure read asked of `$universe`, which offers no head to
	// walk from and requires one (`R-QHEAD`); a claim there is reached by id.
	ErrUniverseNeedsHead = errors.New("ranke/client: a $universe read needs a head")
	// ErrNoUniverse is external content named with no Universe to read the bytes from.
	ErrNoUniverse = errors.New("ranke/client: claims name external content, so a Universe is required")
	// ErrContentMissing is a blob a claim addresses that the Universe does not hold.
	ErrContentMissing = errors.New("ranke/client: the Universe holds no content under this hash")
	// ErrUnknownFraming is a response whose media type names no sequence this can split.
	ErrUnknownFraming = errors.New("ranke/client: response is neither a JSON nor a CBOR sequence")
	// ErrRouteBoundary is a read asking for routes and for claims at once, which the
	// response carries no boundary for (-> routesAreReadable).
	ErrRouteBoundary = errors.New("ranke/client: a read of claims cannot be shaped as paths, the response carrying no route boundary")
)

// DefaultTimeout bounds a request the caller supplied no client for; the generated
// constructor leaves http.Client's zero value, which waits forever.
const DefaultTimeout = 60 * time.Second

// Client talks to one ranke-db instance.
type Client struct {
	base string
	// api parses the JSON routes into typed models, raw hands back the response
	// untouched for the bodies that stream. One transport under both.
	api *api.ClientWithResponses
	raw api.ClientInterface
}

// Option configures a Client.
type Option func(*settings)

// settings collect the options before New resolves them, so exclusivity is decided once
// rather than by whichever option ran last.
type settings struct {
	token    string
	apiKey   string
	macaroon string
	http     *http.Client
}

// WithToken presents a JWT as `Authorization: Bearer`.
func WithToken(t string) Option { return func(s *settings) { s.token = t } }

// WithAPIKey presents a key as `X-API-Key`.
func WithAPIKey(k string) Option { return func(s *settings) { s.apiKey = k } }

// WithMacaroon presents a base64 binary macaroon as `Authorization: Macaroon`. Its own
// option because the endpoint sends every other scheme, Bearer included, to the JWT
// authenticator, where a macaroon 401s with nothing naming the cause.
func WithMacaroon(m string) Option { return func(s *settings) { s.macaroon = m } }

// WithHTTPClient supplies the transport, for timeouts, proxies or a test server. It
// replaces DefaultTimeout.
func WithHTTPClient(h *http.Client) Option { return func(s *settings) { s.http = h } }

// New returns a client for the instance at baseURL, a bare host:port being read as http.
// Several credentials are refused here: the endpoint routes on the scheme presented, so
// two of them name two authenticators. Presenting none reaches a NoAuth endpoint, which
// only a request carrying no credential header at all can.
func New(baseURL string, opts ...Option) (*Client, error) {
	base, err := normalizeBase(baseURL)
	if err != nil {
		return nil, err
	}
	var s settings
	for _, o := range opts {
		o(&s)
	}
	credential, err := s.credential()
	if err != nil {
		return nil, err
	}
	transport := s.http
	if transport == nil {
		transport = &http.Client{Timeout: DefaultTimeout}
	}
	raw, err := api.NewClient(base,
		api.WithHTTPClient(transport),
		api.WithRequestEditorFn(credential))
	if err != nil {
		return nil, err
	}
	// Two views of one transport, so credential and timeout cannot differ between them.
	return &Client{base: base, api: &api.ClientWithResponses{ClientInterface: raw}, raw: raw}, nil
}

// BaseURL is the instance this client addresses, as New resolved it.
func (c *Client) BaseURL() string { return c.base }

// credential renders the credential presented as the editor setting its header.
func (s *settings) credential() (api.RequestEditorFn, error) {
	var named []string
	set := func(header, value string) api.RequestEditorFn {
		return func(_ context.Context, req *http.Request) error {
			req.Header.Set(header, value)
			return nil
		}
	}
	var editor api.RequestEditorFn
	if s.token != "" {
		named, editor = append(named, "token"), set("Authorization", "Bearer "+s.token)
	}
	if s.apiKey != "" {
		named, editor = append(named, "api key"), set("X-API-Key", s.apiKey)
	}
	if s.macaroon != "" {
		named, editor = append(named, "macaroon"), set("Authorization", "Macaroon "+s.macaroon)
	}
	if len(named) > 1 {
		return nil, fmt.Errorf("%w: %s", ErrCredentials, strings.Join(named, " and "))
	}
	if editor == nil {
		return func(context.Context, *http.Request) error { return nil }, nil
	}
	return editor, nil
}

// normalizeBase reads what an operator types: a bare host:port carries no scheme, which
// the generated constructor takes as given and builds an unusable request from.
func normalizeBase(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", ErrNoBaseURL
	}
	if !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("ranke/client: base URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%w: %q names no host", ErrNoBaseURL, raw)
	}
	return strings.TrimRight(s, "/"), nil
}

// accept sets the media types a call will read, which the generated builders leave open.
func accept(types string) api.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("Accept", types)
		return nil
	}
}

// mediaJSON is what a JSON request body is declared as.
const mediaJSON = "application/json"
