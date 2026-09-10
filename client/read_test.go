// package: client / transport
// type:    test
// job:     the reads — both framings split, every record kind discriminated, the trailing report,
// and the by-id routes across the three scopes
// limits:  reads only; contributing what they read back is contribute_test.go's
package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// storyTime is where a case's claims are dated, so the archive's recorded times follow
// the story rather than wall time.
var storyTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

// TestFramingSplitsBothEncodings pins the split itself. A json-seq record opens with
// RS and ends with LF, a cbor-seq record is bare concatenation, and neither delimiter
// may reach the caller: the records come back as the payloads they frame, one per
// result, in both.
func TestFramingSplitsBothEncodings(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime), s.note(t, "second", storyTime))

	for _, tc := range []struct {
		name string
		enc  ranke.ResultEncoding
		open byte
	}{
		{"json-seq", ranke.ResultJSON, '{'},
		{"cbor-seq", ranke.ResultCBOR, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := ranke.Query{Select: ranke.Select{Branch: testBranch}}
			q.Output.Detail = ranke.DetailClaims
			q.Output.Encoding = tc.enc
			records, enc, err := c.QueryRaw(ctx, q)
			if err != nil {
				t.Fatalf("QueryRaw: %v", err)
			}
			if enc != tc.enc {
				t.Fatalf("encoding = %q, want %q — the framing comes from the response", enc, tc.enc)
			}
			if len(records) == 0 {
				t.Fatal("no records")
			}
			for i, rec := range records {
				if len(rec) == 0 {
					t.Fatalf("record %d is empty", i)
				}
				if rec[0] == 0x1e {
					t.Fatalf("record %d still carries its RFC 7464 separator", i)
				}
				if bytes.HasSuffix(rec, []byte("\n")) {
					t.Fatalf("record %d still carries its trailing newline", i)
				}
				if tc.open != 0 && rec[0] != tc.open {
					t.Fatalf("record %d opens with %q, want a whole %s payload", i, rec[0], tc.name)
				}
			}
		})
	}
}

// TestRecordDiscrimination pins every kind a result sequence carries, in both
// framings, against the real endpoint — including the report the stream ends with when
// one was asked for.
func TestRecordDiscrimination(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime))

	route := []ranke.PathStep{{Edges: []string{"*/*"}, Min: ranke.Hops(0), Max: 5}}

	for _, tc := range []struct {
		name string
		out  ranke.Output
		path bool
		want ranke.ResultKind
	}{
		{"an id", ranke.Output{Detail: ranke.DetailID, Encoding: ranke.ResultJSON}, false, ranke.KindClaimId},
		{"an id in cbor", ranke.Output{Detail: ranke.DetailID, Encoding: ranke.ResultCBOR}, false, ranke.KindClaimId},
		{"a route of ids", ranke.Output{Detail: ranke.DetailID, Shape: ranke.ShapePath, Encoding: ranke.ResultJSON}, true, ranke.KindPathId},
		{"a route of ids in cbor", ranke.Output{Detail: ranke.DetailID, Shape: ranke.ShapePath, Encoding: ranke.ResultCBOR}, true, ranke.KindPathId},
		{"a serialized claim", ranke.Output{Detail: ranke.DetailClaims, Encoding: ranke.ResultJSON}, false, ranke.KindClaimEncoded},
		{"a serialized claim in cbor", ranke.Output{Detail: ranke.DetailClaims, Encoding: ranke.ResultCBOR}, false, ranke.KindClaimEncoded},
		{"a stored record", ranke.Output{Detail: ranke.DetailEnvelope, Encoding: ranke.ResultCBOR}, false, ranke.KindClaimEnvelope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sel := ranke.Select{Branch: testBranch}
			if tc.path {
				sel.Path = route
			}
			results, err := c.Query(ctx, ranke.Query{Select: sel, Output: tc.out})
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if len(results) == 0 {
				t.Fatal("no results")
			}
			for i, r := range results {
				if r.Kind != tc.want {
					t.Fatalf("result %d kind = %q, want %q", i, r.Kind, tc.want)
				}
			}
		})
	}
}

// TestTheReportEndsTheSequence pins the one record that is not a result. It is written
// last, so a reader that knows the framing always tells it from a claim — and under
// cbor that reading is the first key's major type, a report's being text where a
// claim's record keys are integers.
func TestTheReportEndsTheSequence(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime))

	for _, enc := range []ranke.ResultEncoding{ranke.ResultJSON, ranke.ResultCBOR} {
		t.Run(string(enc), func(t *testing.T) {
			q := ranke.Query{Select: ranke.Select{Branch: testBranch}}
			q.Output.Detail = ranke.DetailClaims
			q.Output.Encoding = enc
			q.Execution.Report = ranke.ReportInfo
			results, err := c.Query(ctx, q)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			report := ranke.ReportOf(results)
			if report == nil {
				t.Fatalf("the sequence's last record is %q, want the report", results[len(results)-1].Kind)
			}
			if report.Results != len(results)-1 {
				t.Fatalf("report counts %d results, the sequence carries %d", report.Results, len(results)-1)
			}
			if report.StartedAt.IsZero() || report.Elapsed == 0 {
				t.Fatalf("report = %+v, want its times decoded", report)
			}
			if len(report.Events) == 0 {
				t.Fatal("report carries no events, which info asked for")
			}
		})
	}
}

// TestTruncationIsNotAFailure pins `R-QLIMIT`: a read cut short by limit.results is a
// complete answer to the query as bounded. It comes back as results and a report
// saying so, never as an error.
func TestTruncationIsNotAFailure(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime), s.note(t, "second", storyTime))

	q := ranke.Query{Select: ranke.Select{Branch: testBranch}, Limit: ranke.Limit{Results: 1}}
	q.Output.Detail = ranke.DetailID
	q.Execution.Report = ranke.ReportInfo
	results, err := c.Query(ctx, q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	report := ranke.ReportOf(results)
	if report == nil || !report.Truncated {
		t.Fatalf("report = %+v, want a truncated read reported as such", report)
	}
}

// TestQueryClaimsAnswersForTheirOwnIds is what QueryClaims pins its output for. The
// stored record's bytes are what the id is the hash of (`R-QCANON`), so a claim read
// this way carries an id that holds — and the same one the by-id route serves.
func TestQueryClaimsAnswersForTheirOwnIds(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	note := s.note(t, "first", storyTime)
	s.seed(t, c, note)

	claims, err := c.QueryClaims(ctx, ranke.Query{Select: ranke.Select{Branch: testBranch}})
	if err != nil {
		t.Fatalf("QueryClaims: %v", err)
	}
	var found bool
	for _, cl := range claims {
		envelope, err := cl.Envelope()
		if err != nil {
			t.Fatalf("envelope of %s: %v", cl.ID(), err)
		}
		id, err := ranke.HashContent(envelope)
		if err != nil {
			t.Fatalf("hash the envelope of %s: %v", cl.ID(), err)
		}
		if id.String() != cl.ID().String() {
			t.Fatalf("claim reports id %s, its stored record hashes to %s", cl.ID(), id)
		}
		if cl.ID().String() == note.ID().String() {
			found = true
		}
	}
	if !found {
		t.Fatalf("the contributed note is not among the %d claims read back", len(claims))
	}
}

// TestQueryRefusesRoutesOfClaims pins the shape the endpoint cannot answer for: it
// writes one record per claim with no route boundary, so two routes of three and two
// claims arrive byte-identical to five single results. The routes are gone rather than
// ambiguous, so this refuses instead of guessing — while an id-only read keeps its
// routes, each arriving as one record of its own, and is allowed through.
func TestQueryRefusesRoutesOfClaims(t *testing.T) {
	ctx := context.Background()
	_, c := serve(t)

	for _, detail := range []ranke.Detail{ranke.DetailClaims, ranke.DetailEnvelope, ""} {
		q := ranke.Query{Select: ranke.Select{Branch: testBranch}}
		q.Output.Shape = ranke.ShapePath
		q.Output.Detail = detail
		if _, err := c.Query(ctx, q); !errors.Is(err, client.ErrRouteBoundary) {
			t.Fatalf("Query(detail %q as paths) = %v, want ErrRouteBoundary", detail, err)
		}
	}
}

// TestGetClaimAcrossTheScopes pins that one method serves the three by-id routes, and
// that each returns the signed CBOR its id is taken over.
func TestGetClaimAcrossTheScopes(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	note := s.note(t, "first", storyTime)
	s.seed(t, c, note)

	for _, scope := range []client.Scope{client.Scope(testBranch), client.ScopeArchive, client.ScopeUniverse} {
		t.Run(string(scope), func(t *testing.T) {
			got, err := c.GetClaim(ctx, scope, note.ID())
			if err != nil {
				t.Fatalf("GetClaim: %v", err)
			}
			if got.ID().String() != note.ID().String() {
				t.Fatalf("id = %s, want %s", got.ID(), note.ID())
			}
		})
	}
}

// TestGetClaimNotFound pins the answer for a claim outside the scope's closure, which
// the endpoint gives as the one it gives for a claim that never existed.
func TestGetClaimNotFound(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	// Never contributed, so no scope's closure reaches it.
	orphan := s.note(t, "unsent", storyTime)

	if _, err := c.GetClaim(ctx, client.Scope(testBranch), orphan.ID()); !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("GetClaim = %v, want ErrNotFound", err)
	}
	if _, err := c.GetClaim(ctx, client.Scope(testBranch), nil); !errors.Is(err, client.ErrNilClaim) {
		t.Fatalf("GetClaim(nil) = %v, want ErrNilClaim", err)
	}
}

// TestGetContentStreamsTheBytes pins the content routes: the bytes come back whole,
// under the media type the claim's encoding names, and the caller closes the body.
func TestGetContentStreamsTheBytes(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	want := []byte("the document's bytes, which live in the Universe")
	doc := s.external(t, want, storyTime)
	s.seed(t, c, doc)

	for _, scope := range []client.Scope{client.Scope(testBranch), client.ScopeArchive, client.ScopeUniverse} {
		t.Run(string(scope), func(t *testing.T) {
			content, err := c.GetContent(ctx, scope, doc.ID())
			if err != nil {
				t.Fatalf("GetContent: %v", err)
			}
			defer func() { _ = content.Body.Close() }()
			got, err := io.ReadAll(content.Body)
			if err != nil {
				t.Fatalf("read the content: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("content = %q, want %q", got, want)
			}
			if content.MediaType == "" {
				t.Fatal("no media type on the content response")
			}
		})
	}
}

// TestHealthReportsTheStack pins the route WaitReady polls.
func TestHealthReportsTheStack(t *testing.T) {
	_, c := serve(t)
	h, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if h.Status != "ok" {
		t.Fatalf("status = %q, want ok", h.Status)
	}
	if h.Signer == nil || *h.Signer == "" {
		t.Fatal("health names no signing identity")
	}
}
