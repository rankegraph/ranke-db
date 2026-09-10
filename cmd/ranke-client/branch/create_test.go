package branch

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// keypair mints a throwaway contributor key, as --signing-key would load one.
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

// TestTheContributorIsTheWholeContribution: a creation sends the contributor claim and
// nothing else. The branch table is where the creation is recorded, by the Sequencer under
// its own identity, so a claim of the client's saying the same would only restate it.
func TestTheContributorIsTheWholeContribution(t *testing.T) {
	self, err := client.NewContributor(keypair(t))
	if err != nil {
		t.Fatalf("NewContributor: %v", err)
	}
	if got := self.Node().Type(); got != ranke.NodeContributor {
		t.Errorf("the claim sent is %q, want the contributor", got)
	}
	if got := len(self.Edges()); got != 0 {
		t.Errorf("the claim carries %d edges, want none: it cites nothing outside itself, "+
			"which is what keeps the branch independent", got)
	}
	if got := self.Node().Height(); got != 0 {
		t.Errorf("height %d, want 0: an initial claim references nothing (`V-HEIGHT`)", got)
	}
}
