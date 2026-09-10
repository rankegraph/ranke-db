package main

import (
	"context"
	"slices"
	"testing"

	"github.com/rankegraph/ranke-go"
)

// TestIdentityIsReproducible pins what makes a fixture a fixture: the same name always
// derives the same contributor, so re-seeding writes the claims that are already there
// rather than a second set under a new identity.
func TestIdentityIsReproducible(t *testing.T) {
	first, _, err := identity("dev")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	again, _, err := identity("dev")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	if !first.ID().Equal(again.ID()) {
		t.Fatalf("same name derived different ids: %s vs %s", first.ID(), again.ID())
	}

	other, _, err := identity("someone-else")
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	if first.ID().Equal(other.ID()) {
		t.Fatal("different names derived the same identity")
	}
}

// TestExampleGraphHasRealProvenance checks the shape rather than the bytes: one contribution
// per branch, and in the first the four claims whose heights follow from what each cites — 1
// for the sources, 2 for the derivation, 3 for the entity.
func TestExampleGraphHasRealProvenance(t *testing.T) {
	g := newTestGrower(t)
	bs, err := g.example(branchesFor("main", 2))
	if err != nil {
		t.Fatalf("example: %v", err)
	}
	if len(bs) != 2 {
		t.Fatalf("batches = %d, want one per branch", len(bs))
	}
	if bs[0].branch == bs[1].branch {
		t.Fatalf("both contributions name %q — a fixture with one branch tests no branches", bs[0].branch)
	}

	claims := bs[0].claims
	want := []struct {
		typ    string
		height uint64
	}{
		{"source/note", 1},
		{"source/note", 1},
		{"derivation/extraction", 2},
		{"entity/person", 3},
	}
	if len(claims) != len(want) {
		t.Fatalf("claims = %d, want %d", len(claims), len(want))
	}
	for i, w := range want {
		node := claims[i].Node()
		if node.Type() != w.typ {
			t.Errorf("claim %d type = %q, want %q", i, node.Type(), w.typ)
		}
		if node.Height() != w.height {
			t.Errorf("claim %d height = %d, want %d", i, node.Height(), w.height)
		}
	}

	// The derivation cites both sources, which is the provenance the shape exists to show.
	// Which one comes first is the library's — Edges reports canonical order, not the order
	// the builder was handed them — so this asks whether each source is cited, not where.
	cited := inputs(claims[2])
	if len(cited) != 2 {
		t.Fatalf("derivation cites %d inputs, want 2", len(cited))
	}
	for i, source := range claims[:2] {
		if !slices.ContainsFunc(cited, source.ID().Equal) {
			t.Errorf("source %d (%s) is not among the derivation's inputs %v", i, source.ID(), cited)
		}
	}
}

// TestChainGrowsDepth pins that the chain shape produces what it is for: the requested
// batches, and heights that climb past the flat 1 a graph of roots would have.
func TestChainGrowsDepth(t *testing.T) {
	g := newTestGrower(t)
	bs, err := g.chain(10, 6, branchesFor("main", 3))
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if len(bs) != 10 {
		t.Fatalf("batches = %d, want 10", len(bs))
	}

	var deepest uint64
	branches := map[string]int{}
	for i, b := range bs {
		branches[b.branch]++
		if len(b.claims) != 6 {
			t.Fatalf("batch %d holds %d claims, want 6", i, len(b.claims))
		}
		for _, claim := range b.claims {
			deepest = max(deepest, claim.Node().Height())
			// A source is what enters the archive, so it cites nothing but its contributor.
			if isSource(claim.Node().Type()) && len(inputs(claim)) != 0 {
				t.Errorf("%s cites %d inputs, want none", claim.Node().Type(), len(inputs(claim)))
			}
		}
	}
	if deepest < 3 {
		t.Fatalf("deepest height = %d, want a graph with real depth", deepest)
	}
	if len(branches) < 2 {
		t.Fatalf("contributions landed on %d branch(es), want several", len(branches))
	}
}

// TestChainIsDeterministic pins reproducibility across the whole shape, not just the
// identity: two runs of the same command produce the same ids in the same order.
func TestChainIsDeterministic(t *testing.T) {
	first, err := newTestGrower(t).chain(4, 5, branchesFor("main", 3))
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	again, err := newTestGrower(t).chain(4, 5, branchesFor("main", 3))
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	for i := range first {
		if first[i].branch != again[i].branch {
			t.Fatalf("contribution %d landed on %q then %q", i, first[i].branch, again[i].branch)
		}
		for j := range first[i].claims {
			if !first[i].claims[j].ID().Equal(again[i].claims[j].ID()) {
				t.Fatalf("claim %d.%d differs between runs: %s vs %s",
					i, j, first[i].claims[j].ID(), again[i].claims[j].ID())
			}
		}
	}
}

// newTestGrower builds a grower for the fixture identity the tests share.
func newTestGrower(t *testing.T) *grower {
	t.Helper()
	g, err := newGrower(context.Background(), "test")
	if err != nil {
		t.Fatalf("newGrower: %v", err)
	}
	return g
}

// inputs returns the ids a claim cites as derivation inputs, skipping the contributor
// edge every claim carries.
func inputs(claim ranke.Claim) []ranke.Id {
	var ids []ranke.Id
	for _, edge := range claim.Edges() {
		if edge.Type() == "derivation/input" {
			ids = append(ids, edge.Reference())
		}
	}
	return ids
}
