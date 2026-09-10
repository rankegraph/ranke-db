// package: client / transport
// type:    test
// job:     POST /contribute — the round trip, the content gathered from the Universe, and the
// declaration a branch has to be created under
// limits:  the write route; reading back what it merged is read_test.go's
package client_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// TestContributeMergesAndIsIdempotent pins the write and the property the merge is
// content-addressed for: re-contributing yields the same ids and no duplicates.
func TestContributeMergesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	note := s.note(t, "first", storyTime)

	first := s.seed(t, c, note)
	if len(first.Ids) != 2 {
		t.Fatalf("ids = %v, want the contributor and the note", first.Ids)
	}
	if first.Head == "" {
		t.Fatal("the merge reported no new branch-table head")
	}

	again, err := c.Contribute(ctx, s.Universe, testBranch, []ranke.Claim{s.Self, note})
	if err != nil {
		t.Fatalf("contribute again: %v", err)
	}
	if len(again.Ids) != len(first.Ids) {
		t.Fatalf("re-contributing yielded %v, want the same ids as %v", again.Ids, first.Ids)
	}
}

// TestContributeCarriesExternalContent is why Contribute takes a Universe. A claim
// carries only its content_hash, and a re-run's dedup reads that hash rather than
// fetching it, so claims sent alone would leave the bytes behind with nothing
// downstream noticing — the content route is where it would surface, a claim served
// under a hash the archive cannot answer for.
func TestContributeCarriesExternalContent(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	want := []byte("bytes that live outside the claim")
	doc := s.external(t, want, storyTime)
	s.seed(t, c, doc)

	content, err := c.GetContent(ctx, client.Scope(testBranch), doc.ID())
	if err != nil {
		t.Fatalf("GetContent: %v", err)
	}
	defer func() { _ = content.Body.Close() }()
	got, err := io.ReadAll(content.Body)
	if err != nil {
		t.Fatalf("read the content: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content = %q, want %q — the blobs did not travel with the claims", got, want)
	}
}

// TestExternalContentWalksEdgesToo pins that an edge's content is gathered as well: an
// edge holds content of its own, and one missed there is one blob the archive lacks.
func TestExternalContentWalksEdgesToo(t *testing.T) {
	s, c := serve(t)
	body := []byte("what the edge carries")
	hash, err := ranke.HashContent(body)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := s.Universe.PutContents(context.Background(),
		[]ranke.ContentBlob{{Hash: hash, Content: body}}); err != nil {
		t.Fatalf("put content: %v", err)
	}
	edge, err := ranke.NewEdge(ranke.EdgeConfig{
		Reference:   s.Self.ID(),
		Referenced:  s.Self,
		Type:        "derivation/cites",
		ContentHash: hash,
		ContentSize: uint64(len(body)),
		Encoding:    ranke.EncodingText("plain"),
	})
	if err != nil {
		t.Fatalf("edge: %v", err)
	}
	claim := s.claim(t, ranke.NewClaim("entity/note", s.asContributor(t)).
		WithInlineContent([]byte("the node's own content")).
		WithEncoding(ranke.EncodingText("plain")).
		WithCreatedAt(storyTime).
		WithHeight(1).
		WithEdges(edge))

	refs, err := client.ExternalContent([]ranke.Claim{claim})
	if err != nil {
		t.Fatalf("ExternalContent: %v", err)
	}
	if len(refs) != 1 || refs[0].Hash.String() != hash.String() {
		t.Fatalf("refs = %v, want the edge's blob", refs)
	}

	s.seed(t, c, claim)
	content, err := c.GetContent(context.Background(), client.Scope(testBranch), claim.ID())
	if err != nil {
		t.Fatalf("GetContent: %v", err)
	}
	_ = content.Body.Close()
}

// TestContributeRefusesWhatItCannotSend pins the two states the client answers for
// before opening a socket: a claim naming content no Universe was given, and content
// the Universe does not hold.
func TestContributeRefusesWhatItCannotSend(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	doc := s.external(t, []byte("held here"), storyTime)

	if _, err := c.Contribute(ctx, nil, testBranch, []ranke.Claim{doc}); !errors.Is(err, client.ErrNoUniverse) {
		t.Fatalf("Contribute without a Universe = %v, want ErrNoUniverse", err)
	}
	if _, err := c.Contribute(ctx, ranke.NewMemoryUniverse(), testBranch, []ranke.Claim{doc}); !errors.Is(err, client.ErrContentMissing) {
		t.Fatalf("Contribute against an empty Universe = %v, want ErrContentMissing", err)
	}
	if _, err := client.ExternalContent([]ranke.Claim{nil}); !errors.Is(err, client.ErrNilClaim) {
		t.Fatalf("ExternalContent(nil claim) = %v, want ErrNilClaim", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, "", []ranke.Claim{doc}); err == nil {
		t.Fatal("Contribute onto no branch was accepted")
	}
}

// TestCreatingDeclaresANewBranch pins the declaration. The server intersects what it
// permits with what the stream declared, so a contribution that does not ask to create
// cannot create — however the account is granted — and a branch brought into being is
// one the client named.
func TestCreatingDeclaresANewBranch(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	// The archive has to hold the contributor before a second branch can resolve one.
	s.seed(t, c, s.note(t, "first", storyTime))

	const fresh = "release"
	claims := []ranke.Claim{s.Self, s.note(t, "on the new branch", storyTime)}
	if _, err := c.Dev().AdvanceClockPast(ctx, claims); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}

	if _, err := c.Contribute(ctx, s.Universe, fresh, claims); err == nil {
		t.Fatal("a contribution that declared no creation brought a branch into being")
	}

	if _, err := c.Contribute(ctx, s.Universe, fresh, claims, client.Creating()); err != nil {
		t.Fatalf("Contribute(Creating): %v", err)
	}
	branches, err := c.Branches(ctx)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	var held bool
	for _, b := range branches {
		if b.Name == fresh {
			held = true
		}
	}
	if !held {
		t.Fatalf("branches = %v, want %q among them", branches, fresh)
	}
}

// TestContributeOfNothingSendsNothing pins the empty case: no claims is no request, not
// an empty merge that advances the head.
func TestContributeOfNothingSendsNothing(t *testing.T) {
	s, c := serve(t)
	res, err := c.Contribute(context.Background(), s.Universe, testBranch, nil)
	if err != nil {
		t.Fatalf("Contribute(nil): %v", err)
	}
	if res.Head != "" || len(res.Ids) != 0 {
		t.Fatalf("Contribute(nil) = %+v, want nothing merged", res)
	}
}

// TestAClaimBreakingARuleIsRefusedAsInvalid: verification judges the claims submitted, so a
// claim it refuses is answered 400 invalid with the rule named, where it was answered 500
// internal — a status that told a client to retry a fault of the server's rather than to
// correct the claim.
func TestAClaimBreakingARuleIsRefusedAsInvalid(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	// A height no reference supports: the contributor claim sits at 0, so `V-HEIGHT` admits
	// 1 alone, and the server re-derives it.
	wrong := s.claim(t, ranke.NewClaim("entity/note", s.asContributor(t)).
		WithInlineContent([]byte("a claim carrying the wrong height")).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(5))
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{s.Self, wrong}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}

	_, err := c.Contribute(ctx, s.Universe, testBranch, []ranke.Claim{s.Self, wrong})
	if !errors.Is(err, client.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	var refused *client.Error
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want a *client.Error carrying the status", err)
	}
	if refused.Status != 400 {
		t.Errorf("status %d, want 400: the claim is the caller's to correct", refused.Status)
	}
	if !bytes.Contains([]byte(refused.Message), []byte("height")) {
		t.Errorf("the refusal reads %q, and a caller correcting the claim needs the rule "+
			"verification named", refused.Message)
	}
}
