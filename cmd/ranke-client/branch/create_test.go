package branch

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/rankegraph/ranke-go"
)

// keypair mints a throwaway contributor identity, as --signing-key would load one.
func keypair(t *testing.T) ranke.Keypair {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	pubkey, err := ranke.EncodePublicKey(priv.Public())
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return ranke.Keypair{Private: priv, Pubkey: pubkey}
}

// TestClaimsCarryTheContributor: the contributor travels with the creation, because a
// branch holding none admits no writer — every signature resolves through a contributor
// the branch's own closure reaches.
func TestClaimsCarryTheContributor(t *testing.T) {
	built, err := claims("reports", keypair(t))
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	if len(built) != 2 {
		t.Fatalf("got %d claims, want the contributor and the record", len(built))
	}
	if got := built[0].Node().Type(); got != ranke.NodeContributor {
		t.Errorf("first claim is %q, want the contributor to come first", got)
	}
	if got := built[1].Node().Type(); got != TypeBranchCreated {
		t.Errorf("second claim is %q, want %q", got, TypeBranchCreated)
	}
}

// TestContributorIdIsStableAcrossRuns: one key is one identity however often the command
// runs. A contributor claim dated to now would fork the identity on every invocation,
// leaving a branch full of contributors that are all the same person.
func TestContributorIdIsStableAcrossRuns(t *testing.T) {
	pair := keypair(t)
	first, err := claims("reports", pair)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	again, err := claims("reports", pair)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	if first[0].ID().String() != again[0].ID().String() {
		t.Errorf("contributor id moved between runs: %s then %s", first[0].ID(), again[0].ID())
	}
}
