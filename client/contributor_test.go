// package: client / transport
// type:    test
// job:     the contributor claim a key contributes under, and the archive reading them back
// limits:  what a server answers; the key window is contributor_window_test.go's
package client_test

import (
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
	if got := len(client.ContributorsFor([]ranke.Claim{self}, key.Pubkey)); got != 1 {
		t.Error("the claim does not carry its own pubkey")
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

	before, err := c.Contributors(ctx)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	if len(client.ContributorsFor(before, s.Contributor.Pubkey)) != 0 {
		t.Fatal("the case's key is present before it contributed anything")
	}

	s.seed(t, c, s.note(t, "contributed", storyTime))

	after, err := c.Contributors(ctx)
	if err != nil {
		t.Fatalf("Contributors: %v", err)
	}
	if got := len(client.ContributorsFor(after, s.Contributor.Pubkey)); got != 1 {
		t.Errorf("the contributed key has %d claims among the archive's %d, want 1",
			got, len(after))
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
