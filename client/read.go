// package: client / transport
// type:    adapter
// job:     the reads — POST /query in either framing, the by-id claim and content routes across
// the three scopes, and /health with the wait a caller would otherwise spend on a guessed sleep
// limits:  transport only; the read language is the library's (-> ranke-go query_codec) and
// splitting the answer is seq.go's
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rankegraph/ranke-go"
)

// Scope names which of the three by-id routes a read takes: a branch, or one of the two
// reserved scopes.
type Scope string

// The reserved scopes. Any other value is a branch name.
const (
	ScopeArchive  Scope = "$archive"
	ScopeUniverse Scope = "$universe"
)

// Health reports the instance as up, the identity it merges under, and its build.
func (c *Client) Health(ctx context.Context) (*Health, error) {
	res, err := c.api.HealthWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}

// readyGap paces WaitReady's polling of a stack still coming up.
const readyGap = 50 * time.Millisecond

// WaitReady polls /health until it answers or within elapses, so a caller starting a
// stack waits for the fact rather than a guessed interval. A settled refusal ends the
// wait at once, retrying an answer already given only spending the window.
func (c *Client) WaitReady(ctx context.Context, within time.Duration) error {
	deadline := time.Now().Add(within)
	var last error
	for {
		_, err := c.Health(ctx)
		if err == nil {
			return nil
		}
		var refused *Error
		if errors.As(err, &refused) && settled(refused) {
			return err
		}
		last = err
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !time.Now().Add(readyGap).Before(deadline) {
			return fmt.Errorf("ranke/client: no answer from %s/health within %s: %w", c.base, within, last)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(readyGap):
		}
	}
}

// settled reports whether a refusal is one waiting cannot change — a credential the
// stack will not accept, a route it does not mount. A stack with no free slot (429) and
// a proxy answering for a backend still coming up (5xx) are both worth another poll,
// which is the case WaitReady exists for.
func settled(e *Error) bool {
	return !errors.Is(e, ErrBusy) && e.Status < http.StatusInternalServerError
}

// QueryRaw returns each record of the result sequence as bytes, split by the framing the
// response declares. It names no payload, so a result kind added upstream needs no
// release here.
func (c *Client) QueryRaw(ctx context.Context, q ranke.Query) ([][]byte, ranke.ResultEncoding, error) {
	body, err := ranke.EncodeQuery(q)
	if err != nil {
		return nil, "", err
	}
	resp, err := c.raw.QueryWithBody(ctx, mediaJSON, bytes.NewReader(body),
		accept(MediaJSONSeq+", "+MediaCBORSeq))
	if err != nil {
		return nil, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errorFromBody(resp)
	}
	return splitBody(resp)
}

// Query returns q's results, each record read back into the QueryResult ranke-go
// describes it with, a reported run ending with the KindReport element ranke.ReportOf
// reads off the tail. A read cut short by a limit is a complete answer to the query as
// bounded (`R-QLIMIT`), so truncation stays in the report rather than becoming an error.
func (c *Client) Query(ctx context.Context, q ranke.Query) ([]ranke.QueryResult, error) {
	if err := routesAreReadable(q.Output); err != nil {
		return nil, err
	}
	records, enc, err := c.QueryRaw(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]ranke.QueryResult, 0, len(records))
	for _, raw := range records {
		result, err := decodeRecord(raw, enc)
		if err != nil {
			return nil, err
		}
		out = append(out, result)
	}
	return out, nil
}

// QueryClaims shapes q so every record is a stored record, and decodes each. The
// envelope in CBOR is fixed here, being the bytes whose hash is the id (`R-QCANON`);
// a serialized claim is a rendering no id covers, and `R-QDETAIL` refuses json outright.
func (c *Client) QueryClaims(ctx context.Context, q ranke.Query) ([]ranke.Claim, error) {
	q.Output.Detail = ranke.DetailEnvelope
	q.Output.Form = ranke.FormOriginal
	q.Output.Encoding = ranke.ResultCBOR
	q.Output.Content = nil
	q.Execution.Report = ""

	results, err := c.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	claims := make([]ranke.Claim, 0, len(results))
	for _, r := range results {
		if r.Kind != ranke.KindClaimEnvelope {
			return nil, ranke.WithDetail(ErrUnknownFraming, "expected a stored record, got "+string(r.Kind))
		}
		claims = append(claims, r.ClaimNative)
	}
	return claims, nil
}

// routesAreReadable refuses a read of claims shaped as paths: the endpoint writes one
// record per claim with no boundary, so two routes of three and two arrive identical to
// five single results. An id-only read keeps its routes, one record each.
func routesAreReadable(out ranke.Output) error {
	if out.Shape == ranke.ShapePath && out.Detail != ranke.DetailID {
		return ErrRouteBoundary
	}
	return nil
}

// GetClaim fetches one claim by id within scope, as the signed CBOR its id is taken
// over. A claim outside that closure is answered as one that never existed.
func (c *Client) GetClaim(ctx context.Context, scope Scope, id ranke.Id) (ranke.Claim, error) {
	if id == nil {
		return nil, ErrNilClaim
	}
	var body []byte
	var status int
	switch scope {
	case ScopeArchive:
		res, err := c.api.GetArchiveClaimWithResponse(ctx, id.String())
		if err != nil {
			return nil, err
		}
		body, status = res.Body, res.StatusCode()
	case ScopeUniverse:
		res, err := c.api.GetClaimWithResponse(ctx, id.String())
		if err != nil {
			return nil, err
		}
		body, status = res.Body, res.StatusCode()
	default:
		res, err := c.api.GetBranchClaimWithResponse(ctx, string(scope), id.String())
		if err != nil {
			return nil, err
		}
		body, status = res.Body, res.StatusCode()
	}
	if status != http.StatusOK {
		return nil, refusal(status, body)
	}
	return ranke.DecodeClaim(id, body)
}

// Content is a claim's content as the server streams it. The caller closes Body.
type Content struct {
	// MediaType is the claim's encoding, as the endpoint declares it.
	MediaType string
	Body      io.ReadCloser
}

// GetContent streams the content of claim id within scope. Content is addressed by the
// claim rather than by a raw hash, so the read stays scoped as the route is. The body is
// handed over unread: buffering arbitrary bytes decides what a caller can afford.
func (c *Client) GetContent(ctx context.Context, scope Scope, id ranke.Id) (*Content, error) {
	if id == nil {
		return nil, ErrNilClaim
	}
	var resp *http.Response
	var err error
	switch scope {
	case ScopeArchive:
		resp, err = c.raw.GetArchiveClaimContent(ctx, id.String())
	case ScopeUniverse:
		resp, err = c.raw.GetClaimContent(ctx, id.String())
	default:
		resp, err = c.raw.GetBranchClaimContent(ctx, string(scope), id.String())
	}
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, errorFromBody(resp)
	}
	return &Content{MediaType: resp.Header.Get("Content-Type"), Body: resp.Body}, nil
}

// errorFromBody reads a refusal off a response the plain client returned unparsed.
func errorFromBody(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	if err != nil {
		return refusal(resp.StatusCode, nil)
	}
	return refusal(resp.StatusCode, body)
}
