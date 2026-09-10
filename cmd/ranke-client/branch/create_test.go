package branch

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
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

// registered builds the identity a resolve against a founded archive answers with: a
// contributor claim the server attested, so it carries a contributor edge of its own and
// sits above the initial claim it resolves through. Nothing a client mints looks like
// this, which is why it can never be the same claim as one minted here.
func registered(t *testing.T) *client.Contributor {
	t.Helper()
	ctx := context.Background()
	root := keypair(t)
	rootClaim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(root.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		Sign(root.Private)
	if err != nil {
		t.Fatalf("sign the root: %v", err)
	}
	rootAs, err := rootClaim.AsContributor(ctx, nil, root.Private)
	if err != nil {
		t.Fatalf("bind the root: %v", err)
	}
	pair := keypair(t)
	claim, err := ranke.NewClaim(ranke.NodeContributor, rootAs).
		WithInlineContent(pair.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(1).
		Sign()
	if err != nil {
		t.Fatalf("sign the contributor: %v", err)
	}
	as, err := claim.AsContributor(ctx, nil, pair.Private)
	if err != nil {
		t.Fatalf("bind the contributor: %v", err)
	}
	return &client.Contributor{Claim: claim, As: as, Registered: true}
}

// TestARegistrationCarriesTheContributor: a key contributing for the first time has no
// claim in the archive to reference, so its contributor claim travels with the creation —
// every signature resolves through a contributor the closure reaches (`V-SIG`).
func TestARegistrationCarriesTheContributor(t *testing.T) {
	self, err := client.RegisterContributor(keypair(t))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	built, err := claims("reports", self)
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

// TestARegisteredContributorIsReferenced is the rule this command exists to keep: a pubkey
// the archive already holds is referenced, never contributed a second time. A second claim
// over one key is a second identity, and nothing can retract it (`R-DSTRUCT`).
func TestARegisteredContributorIsReferenced(t *testing.T) {
	self := registered(t)
	built, err := claims("reports", self)
	if err != nil {
		t.Fatalf("claims: %v", err)
	}
	if len(built) != 1 {
		t.Fatalf("got %d claims, want the record alone", len(built))
	}
	if got := built[0].Node().Type(); got != TypeBranchCreated {
		t.Errorf("the one claim is %q, want %q", got, TypeBranchCreated)
	}
	for _, c := range built {
		if c.Node().Type() == ranke.NodeContributor {
			t.Error("the contributor claim was contributed again")
		}
	}
	if got := built[0].Contributor().ID().String(); got != self.Claim.ID().String() {
		t.Errorf("the record is attributed to %s, want the registered %s", got, self.Claim.ID())
	}
}

// TestTheRecordClimbsPastItsContributor: height is the longest path along references
// (`V-HEIGHT`), so a record citing an attested contributor sits above it rather than at
// the 1 a self-registered contributor's record sits at.
func TestTheRecordClimbsPastItsContributor(t *testing.T) {
	fresh, err := client.RegisterContributor(keypair(t))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	for _, tc := range []struct {
		name string
		self *client.Contributor
		want uint64
	}{
		{"self-registered", fresh, 1},
		{"attested by the server", registered(t), 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			built, err := claims("reports", tc.self)
			if err != nil {
				t.Fatalf("claims: %v", err)
			}
			record := built[len(built)-1]
			if got := record.Node().Height(); got != tc.want {
				t.Errorf("record height %d, want %d — one past the contributor at %d",
					got, tc.want, tc.self.Claim.Node().Height())
			}
		})
	}
}

// TestContributorIdIsStableAcrossRuns: one key is one identity however often the command
// runs. A contributor claim dated to now would fork the identity on every invocation,
// leaving an archive full of contributors that are all the same person.
func TestContributorIdIsStableAcrossRuns(t *testing.T) {
	pair := keypair(t)
	first, err := client.RegisterContributor(pair)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	again, err := client.RegisterContributor(pair)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if first.Claim.ID().String() != again.Claim.ID().String() {
		t.Errorf("contributor id moved between runs: %s then %s", first.Claim.ID(), again.Claim.ID())
	}
}
