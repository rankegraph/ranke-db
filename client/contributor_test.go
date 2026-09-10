// package: client / transport
// type:    test
// job:     the contributor claim a key contributes under, admitting a second key to a branch,
// and the archive reading them back
// limits:  what a server answers; the key window is contributor_window_test.go's
package client_test

import (
	"bytes"
	"context"
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

// TestNewContributorCarriesTheKey: the claim states the pubkey and is bound to the key so it
// signs at once, and it is dated when it was built — `created_at` is the moment the claim was
// added and nothing else (paper 01 §Nodes).
func TestNewContributorCarriesTheKey(t *testing.T) {
	key := newKeypair(t)
	before := time.Now().UTC().Add(-time.Second)
	self, err := client.NewContributor(key)
	if err != nil {
		t.Fatalf("NewContributor: %v", err)
	}
	pubkey, err := self.Node().GetInlineContent()
	if err != nil || !bytes.Equal(pubkey, key.Pubkey) {
		t.Errorf("the claim carries %x, want its own pubkey %x (%v)", pubkey, key.Pubkey, err)
	}
	if self.SigningKey() == nil {
		t.Error("no signing key bound, so nothing could be signed under it")
	}
	if got := self.Node().Height(); got != 0 {
		t.Errorf("height %d, want 0: a contributor claim references nothing", got)
	}
	if at := self.Node().CreatedAt(); at.Before(before) {
		t.Errorf("dated %s, before this test started — it should carry the moment it was built", at)
	}
}

// TestEachBranchHoldsItsOwnContributor: a creation contributes one claim carrying the
// pubkey, and it becomes that branch's head. Two branches hold one each, neither referencing
// the other, which is what leaves each an independent graph.
func TestEachBranchHoldsItsOwnContributor(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	for _, branch := range []string{"reports", "audits"} {
		self, err := client.NewContributor(s.Contributor)
		if err != nil {
			t.Fatalf("NewContributor: %v", err)
		}
		if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{self}); err != nil {
			t.Fatalf("advance the dev clock: %v", err)
		}
		// Referencing nothing: each branch is self-contained, so no read of another is
		// needed and none of the $-targets a tenant may not hold is either.
		if _, err := c.Contribute(ctx, s.Universe, branch, []ranke.Claim{self},
			client.Creating(), client.Referencing()); err != nil {
			t.Fatalf("create %q: %v", branch, err)
		}
		head, err := c.BranchHead(ctx, branch)
		if err != nil {
			t.Fatalf("head of %q: %v", branch, err)
		}
		if head != self.ID().String() {
			t.Errorf("branch %q heads at %s, want the contributor claim %s", branch, head, self.ID())
		}
	}
}

// TestAContributionKeepsWhatTheBranchHeld: the Sequencer folds a branch's current head with
// what a contribution brings, so a claim citing nothing does not cost the branch what it
// pointed at — both are consolidated under a new head instead.
func TestAContributionKeepsWhatTheBranchHeld(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	self, err := client.NewContributor(s.Contributor)
	if err != nil {
		t.Fatalf("NewContributor: %v", err)
	}
	first := s.claim(t, ranke.NewClaim("entity/note", self).
		WithInlineContent([]byte("held")).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(1))
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{self, first}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, "reports", []ranke.Claim{self, first},
		client.Creating(), client.Referencing()); err != nil {
		t.Fatalf("create: %v", err)
	}

	// A second claim citing only its contributor — nothing the branch already holds.
	second := s.claim(t, ranke.NewClaim("entity/note", self).
		WithInlineContent([]byte("loose")).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(1))
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{second}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, "reports", []ranke.Claim{second},
		client.Referencing()); err != nil {
		t.Fatalf("contribute the second claim: %v", err)
	}
	for _, want := range []ranke.Claim{first, second} {
		if _, err := c.GetClaim(ctx, client.Scope("reports"), want.ID()); err != nil {
			t.Errorf("%s is not in the branch after the second contribution: %v", want.ID(), err)
		}
	}
}

// TestContributorsReadsThemBack: the archive's registrations are what `contributor list`
// reports, and the read is scoped to `$archive`, so it needs R there.
func TestContributorsReadsThemBack(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	before, err := c.ContributorsFor(ctx, client.ScopeArchive, s.Contributor.Pubkey)
	if err != nil {
		t.Fatalf("ContributorsFor: %v", err)
	}
	if len(before) != 0 {
		t.Fatal("the case's key is present before it contributed anything")
	}

	s.seed(t, c, s.note(t, "contributed", storyTime))

	after, err := c.Contributors(ctx, client.ScopeArchive)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	mine, err := c.ContributorsFor(ctx, client.ScopeArchive, s.Contributor.Pubkey)
	if err != nil {
		t.Fatalf("ContributorsFor: %v", err)
	}
	if len(mine) != 1 {
		t.Errorf("the contributed key has %d claims among the archive's %d, want 1",
			len(mine), len(after))
	}
	for _, c := range after {
		if got := c.Node().Type(); got != ranke.NodeContributor {
			t.Errorf("the read returned a %q, want contributor claims alone", got)
		}
	}
}

// expiryAgainst builds the limiting claim that retires the contributor at target from at: a
// `contribution/expiry` claim whose edge carries the date, `R-DEXPIRY` putting it on the edge
// rather than the claim. The Sequencer alone creates one (`R-C2TYPE`); this stands in.
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
	self, err := client.NewContributor(by)
	if err != nil {
		t.Fatalf("NewContributor: %v", err)
	}
	claim, err := ranke.NewClaim(expiryType, self).
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

// TestAKeyIsAdmittedByOneTheBranchHolds: the second writer arrives as a contributor claim
// carrying its pubkey, attributed to a contributor the branch already holds and signed under
// that key. The newcomer then signs its own claims onto the branch, each resolving through the
// claim that admitted it (`V-SIG`) — which is what a branch admitting no writer of its own
// could not do.
func TestAKeyIsAdmittedByOneTheBranchHolds(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	const branch = "reports"

	founder, err := client.NewContributor(s.Contributor)
	if err != nil {
		t.Fatalf("NewContributor: %v", err)
	}
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{founder}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, branch, []ranke.Claim{founder},
		client.Creating(), client.Referencing()); err != nil {
		t.Fatalf("create %q: %v", branch, err)
	}

	// The claim the admission is attributed to, as a writer finds it: read back from the
	// branch, bound to the key it carries, which needs R on the branch and no `$`-target.
	mine, err := c.ContributorsFor(ctx, client.Scope(branch), s.Contributor.Pubkey)
	if err != nil {
		t.Fatalf("ContributorsFor: %v", err)
	}
	if len(mine) != 1 {
		t.Fatalf("the branch holds %d claims for the founding key, want 1", len(mine))
	}
	signing, err := mine[0].AsContributor(ctx, nil, s.Contributor.Private)
	if err != nil {
		t.Fatalf("AsContributor: %v", err)
	}

	// The admission itself, as an application builds one: the newcomer's pubkey stated by a
	// claim attributed to the contributor the branch already holds.
	newcomer := newKeypair(t)
	admission, err := ranke.NewClaim(ranke.NodeContributor, signing).
		WithInlineContent(newcomer.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(ranke.HeightOf(signing)).
		Sign()
	if err != nil {
		t.Fatalf("sign the admission: %v", err)
	}
	if got := admission.Node().Height(); got != signing.Node().Height()+1 {
		t.Errorf("height %d, want one above the %d of the contributor it is attributed to "+
			"(`V-HEIGHT`)", got, signing.Node().Height())
	}
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{admission}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, branch, []ranke.Claim{admission}); err != nil {
		t.Fatalf("admit the newcomer: %v", err)
	}

	admitted, err := c.ContributorsFor(ctx, client.Scope(branch), newcomer.Pubkey)
	if err != nil {
		t.Fatalf("ContributorsFor: %v", err)
	}
	if len(admitted) != 1 {
		t.Fatalf("the branch holds %d claims for the admitted key, want 1", len(admitted))
	}

	// The point of the admission: the newcomer's own key now writes to the branch.
	as, err := admission.AsContributor(ctx, nil, newcomer.Private)
	if err != nil {
		t.Fatalf("AsContributor for the newcomer: %v", err)
	}
	note, err := ranke.NewClaim("entity/note", as).
		WithInlineContent([]byte("written by the admitted key")).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(ranke.HeightOf(as)).
		Sign()
	if err != nil {
		t.Fatalf("sign under the admitted key: %v", err)
	}
	// Two above the founding claim: an admitted key's own claims sit above the admission,
	// so a client hardcoding 1 here is refused by the verifier (`V-HEIGHT`).
	if got := note.Node().Height(); got != 2 {
		t.Errorf("a claim under the admitted key sits at height %d, want 2", got)
	}
	if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{note}); err != nil {
		t.Fatalf("advance the dev clock: %v", err)
	}
	if _, err := c.Contribute(ctx, s.Universe, branch, []ranke.Claim{note}); err != nil {
		t.Fatalf("contribute under the admitted key: %v", err)
	}
	if _, err := c.GetClaim(ctx, client.Scope(branch), note.ID()); err != nil {
		t.Errorf("%s is not on the branch: %v", note.ID(), err)
	}
}

// TestABranchListsOnlyItsOwnContributors: the branch-scoped read answers for one closure, so a
// key one branch admits is absent from another that never admitted it. `$archive` is the read
// that spans them, and needs R on a `$`-target no tenant holds.
func TestABranchListsOnlyItsOwnContributors(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	keys := map[string]ranke.Keypair{"reports": newKeypair(t), "audits": newKeypair(t)}

	for branch, key := range keys {
		self, err := client.NewContributor(key)
		if err != nil {
			t.Fatalf("NewContributor: %v", err)
		}
		if _, err := c.Dev().AdvanceClockPast(ctx, []ranke.Claim{self}); err != nil {
			t.Fatalf("advance the dev clock: %v", err)
		}
		if _, err := c.Contribute(ctx, s.Universe, branch, []ranke.Claim{self},
			client.Creating(), client.Referencing()); err != nil {
			t.Fatalf("create %q: %v", branch, err)
		}
	}

	for branch, key := range keys {
		own, err := c.ContributorsFor(ctx, client.Scope(branch), key.Pubkey)
		if err != nil {
			t.Fatalf("ContributorsFor(%q): %v", branch, err)
		}
		if len(own) != 1 {
			t.Errorf("branch %q holds %d claims for its own key, want 1", branch, len(own))
		}
		for other, elsewhere := range keys {
			if other == branch {
				continue
			}
			foreign, err := c.ContributorsFor(ctx, client.Scope(branch), elsewhere.Pubkey)
			if err != nil {
				t.Fatalf("ContributorsFor(%q): %v", branch, err)
			}
			if len(foreign) != 0 {
				t.Errorf("branch %q holds %d claims for %q's key, want none",
					branch, len(foreign), other)
			}
		}
		// No limiting claim was made against either key, so the branch binds no window.
		if expiries, err := c.Expiries(ctx, client.Scope(branch)); err != nil {
			t.Errorf("Expiries(%q): %v", branch, err)
		} else if len(expiries) != 0 {
			t.Errorf("branch %q holds %d expiries, want none", branch, len(expiries))
		}
	}

	for branch, key := range keys {
		all, err := c.ContributorsFor(ctx, client.ScopeArchive, key.Pubkey)
		if err != nil {
			t.Fatalf("ContributorsFor($archive): %v", err)
		}
		if len(all) != 1 {
			t.Errorf("the archive holds %d claims for %q's key, want 1", len(all), branch)
		}
	}
}
