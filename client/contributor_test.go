// package: client / transport
// type:    test
// job:     resolving a signing key to its contributor — the read behind it, the choice among
// several claims, and the contribution that references one instead of sending it again
// limits:  the decision and what a server answers; the window is contributor_window_test.go's
package client_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// newKeypair mints a throwaway contributor key, as an application would hold one.
func newKeypair(t *testing.T) ranke.Keypair {
	t.Helper()
	pair, _ := contributor(t)
	return pair
}

// expiryAgainst builds the limiting claim that retires the contributor at target from at:
// a `contribution/expiry` claim whose edge carries the date (`R-DEXPIRY` puts it on the
// edge, not the claim).
func expiryAgainst(t *testing.T, by ranke.Keypair, target ranke.Id, at time.Time) ranke.Claim {
	t.Helper()
	edge, err := ranke.NewEdge(ranke.EdgeConfig{
		Reference: target,
		Type:      ranke.EdgeTypeExpiry,
		Fields:    map[string]string{ranke.FieldPubkeyExpiresAfter: ranke.FormatTimestamp(at)},
	})
	if err != nil {
		t.Fatalf("expiry edge: %v", err)
	}
	// The Sequencer alone creates a limiting claim (`R-C2TYPE`); this stands in for one, so
	// it is attributed like any other claim rather than standing as a root.
	claim, err := ranke.NewClaim(expiryType, rootOf(t, by)).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		WithHeight(1).
		WithEdges(edge).
		Sign()
	if err != nil {
		t.Fatalf("sign the expiry: %v", err)
	}
	return claim
}

// expiryType is the limiting claim's own type.
var expiryType = string(ranke.NodeClassContribution) + "/" + string(ranke.NodeSubtypeExpiry)

// rootOf binds key's own root contributor claim, for attributing a claim built by hand.
func rootOf(t *testing.T, key ranke.Keypair) ranke.Contributor {
	t.Helper()
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(key.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		Sign(key.Private)
	if err != nil {
		t.Fatalf("sign a root contributor: %v", err)
	}
	as, err := claim.AsContributor(context.Background(), nil, key.Private)
	if err != nil {
		t.Fatalf("bind a root contributor: %v", err)
	}
	return as
}

// TestContributorsHoldsTheRegisteredKeys: a pubkey enters the graph once and is referenced
// from then on, which needs a way to ask what the archive holds for a key. The founding
// contributor and the sequencer's own are there from genesis; a contributed one joins them.
func TestContributorsHoldsTheRegisteredKeys(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	before, err := c.Contributors(ctx)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	if len(client.ContributorsFor(before, s.Contributor.Pubkey)) != 0 {
		t.Fatal("the case's key is registered before it contributed anything")
	}

	s.seed(t, c, s.note(t, "registered", storyTime))

	after, err := c.Contributors(ctx)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	if got := len(client.ContributorsFor(after, s.Contributor.Pubkey)); got != 1 {
		t.Errorf("the contributed key has %d contributor claims among the archive's %d, want 1",
			got, len(after))
	}
	for _, held := range after {
		if got := held.Node().Type(); got != ranke.NodeContributor {
			t.Errorf("the read returned a %q, want contributor claims alone", got)
		}
	}
}

// TestResolveNeedsTheArchiveRead: the read is scoped to `$archive`, so an account without R
// there is refused rather than answered with an empty list — the answer a caller would
// otherwise read as "this key is new" and register a second contributor on.
func TestResolveNeedsTheArchiveRead(t *testing.T) {
	s, c := serve(t, withGrants("CR *", "C $branches", "R $branches"))
	_, err := c.ResolveContributor(context.Background(), s.Contributor, storyTime, nil)
	if !errors.Is(err, client.ErrContributorUnresolved) {
		t.Fatalf("resolve without R $archive = %v, want client.ErrContributorUnresolved", err)
	}
	if !errors.Is(err, client.ErrForbidden) {
		t.Errorf("the refusal should carry the server's category too: %v", err)
	}
}

// TestResolveRegistersAKeyTheArchiveDoesNotHold: no claim over the pubkey is the one case
// where minting is right, and the caller is told so, since that claim has to travel.
func TestResolveRegistersAKeyTheArchiveDoesNotHold(t *testing.T) {
	s, c := serve(t)
	self, err := c.ResolveContributor(context.Background(), s.Contributor, storyTime, nil)
	if err != nil {
		t.Fatalf("ResolveContributor: %v", err)
	}
	if self.Registered {
		t.Error("Registered is set for a key the archive does not hold")
	}
	if self.Claim.ID().String() != s.Self.ID().String() {
		t.Errorf("minted %s, want the same id the harness mints for this key, %s",
			self.Claim.ID(), s.Self.ID())
	}
}

// TestResolveTakesTheRegisteredClaim: once the archive holds the key, a resolve returns that
// claim rather than a fresh one — the rule the whole file exists for.
func TestResolveTakesTheRegisteredClaim(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime))

	self, err := c.ResolveContributor(ctx, s.Contributor, storyTime, nil)
	if err != nil {
		t.Fatalf("ResolveContributor: %v", err)
	}
	if !self.Registered {
		t.Fatal("Registered is not set for a key the archive holds")
	}
	if self.As == nil {
		t.Error("no bound contributor, so nothing could be signed under it")
	}
	if !self.Window.Admits(storyTime) {
		t.Errorf("the window %s does not admit the date resolved for", self.Window)
	}
}

// TestResolveRefusesALapsedKey: `R-C4KEY` admits only a claim whose window contains the
// date, so a resolve refuses here rather than sending a contribution the server rejects at
// step 4. It names rotation, that being the way out.
func TestResolveRefusesALapsedKey(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	lapsing, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(s.Contributor.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		WithField(ranke.FieldPubkeyExpiresAfter, ranke.FormatTimestamp(storyTime.Add(-time.Hour))).
		Sign(s.Contributor.Private)
	if err != nil {
		t.Fatalf("sign the lapsing contributor: %v", err)
	}
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{lapsing}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, testBranch, []ranke.Claim{lapsing}); err != nil {
		t.Fatalf("contribute the lapsing contributor: %v", err)
	}

	_, err = c.ResolveContributor(ctx, s.Contributor, storyTime, nil)
	if !errors.Is(err, client.ErrContributorLapsed) {
		t.Fatalf("resolve past the window = %v, want client.ErrContributorLapsed", err)
	}
}

// TestResolveRefusesAnAmbiguousKey: two claims valid at once is the state a second
// registration leaves, and nothing retracts it (`R-DSTRUCT`). There is no right one to pick,
// so the refusal names both and the caller settles it.
func TestResolveRefusesAnAmbiguousKey(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	// A second claim over the same pubkey, differing only in when it says it was added —
	// which is exactly how two writers dating an identity differently fork one.
	second, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(s.Contributor.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(storyTime.Add(-time.Hour)).
		Sign(s.Contributor.Private)
	if err != nil {
		t.Fatalf("sign the second contributor: %v", err)
	}
	s.seed(t, c, second)

	_, err = c.ResolveContributor(ctx, s.Contributor, storyTime, nil)
	if !errors.Is(err, client.ErrContributorAmbiguous) {
		t.Fatalf("resolve of a forked key = %v, want client.ErrContributorAmbiguous", err)
	}

	// Named, it resolves — and to the one named, not to whichever came back first.
	picked, err := c.ResolveContributor(ctx, s.Contributor, storyTime, second.ID())
	if err != nil {
		t.Fatalf("ResolveContributor(pick): %v", err)
	}
	if picked.Claim.ID().String() != second.ID().String() {
		t.Errorf("picked %s, want the named %s", picked.Claim.ID(), second.ID())
	}
	// A claim the key does not carry is refused rather than silently ignored.
	if _, err := c.ResolveContributor(ctx, s.Contributor, storyTime, s.note(t, "not a contributor", storyTime).ID()); !errors.Is(err, client.ErrNoSuchContributor) {
		t.Errorf("resolve naming a foreign claim = %v, want client.ErrNoSuchContributor", err)
	}
}

// TestAContributorIsReferencedAcrossBranches is the path a second branch takes once a key is
// registered: the claim cites the contributor where it stands, the contribution declares the
// archive citeable, and the server draws the claim in as it closes (`R-C3CLOSE`). No second
// contributor claim is sent, and the merge stands.
func TestAContributorIsReferencedAcrossBranches(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "first", storyTime))

	self, err := c.ResolveContributor(ctx, s.Contributor, storyTime, nil)
	if err != nil {
		t.Fatalf("ResolveContributor: %v", err)
	}
	record := s.claim(t, ranke.NewClaim("contribution/branch_created", self.As).
		WithInlineContent([]byte("reports")).
		WithEncoding(ranke.EncodingText("plain")).
		WithCreatedAt(storyTime).
		WithHeight(self.Claim.Node().Height()+1))
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{record}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}

	res, err := c.Contribute(ctx, s.Universe, "reports", []ranke.Claim{record},
		client.Creating(), client.Referencing(ranke.BranchArchive))
	if err != nil {
		t.Fatalf("contribute the record alone: %v", err)
	}
	if len(res.Ids) != 1 {
		t.Errorf("the merge absorbed %d claims, want the record alone", len(res.Ids))
	}
	// The referenced contributor is in the new branch's closure, which is what makes the
	// record's signature resolve there (`V-SIG`).
	if _, err := c.GetClaim(ctx, client.Scope("reports"), self.Claim.ID()); err != nil {
		t.Errorf("the contributor is not in the new branch's closure: %v", err)
	}
	// And it is still the one claim over that key: referencing did not add a second.
	again, err := c.Contributors(ctx)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	if got := len(client.ContributorsFor(again, s.Contributor.Pubkey)); got != 1 {
		t.Errorf("the key now has %d contributor claims, want the one it was registered with", got)
	}
}
