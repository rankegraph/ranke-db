package contributor

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// aKey mints a throwaway contributor key.
func aKey(t *testing.T) ranke.Keypair {
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

// keyed builds a contributor claim over key, dated at so two claims over one key differ —
// which is how a second registration forks an identity in the first place.
func keyed(t *testing.T, key ranke.Keypair, at time.Time) ranke.Claim {
	t.Helper()
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(key.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(at).
		Sign(key.Private)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return claim
}

// TestSharingNamesOnlyTheForkedKeys is what makes a forked identity visible: one claim per
// pubkey is the ordinary case and says nothing, two over one pubkey is the state a second
// registration leaves and nothing can retract it (`R-DSTRUCT`).
func TestSharingNamesOnlyTheForkedKeys(t *testing.T) {
	forked := aKey(t)
	epoch := time.Unix(0, 0).UTC()
	alone := keyed(t, aKey(t), epoch)
	first := keyed(t, forked, epoch)
	second := keyed(t, forked, epoch.Add(time.Hour))

	shared := sharing([]ranke.Claim{alone, first, second})
	if len(shared) != 1 {
		t.Fatalf("%d pubkeys reported as shared, want the one carried twice", len(shared))
	}
	ids := shared[string(forked.Pubkey)]
	if len(ids) != 2 {
		t.Fatalf("the forked key names %d claims, want both", len(ids))
	}
	if first.ID().String() == second.ID().String() {
		t.Fatal("the two claims have one id, so this case proves nothing")
	}
}

// TestStatusReadsTheWindowAtTheDate: the listing answers the question `R-C4KEY` asks — would
// a claim signed now verify under this contributor — rather than printing bounds alone.
func TestStatusReadsTheWindowAtTheDate(t *testing.T) {
	at := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	future := at.AddDate(1, 0, 0)
	past := at.AddDate(-1, 0, 0)
	for _, tc := range []struct {
		name string
		w    client.ContributorWindow
		want string
	}{
		{"open at both ends", client.ContributorWindow{}, "valid now"},
		{"opens later", client.ContributorWindow{From: &future}, "not yet valid"},
		{"closed already", client.ContributorWindow{Until: &past}, "lapsed"},
		{"inside", client.ContributorWindow{From: &past, Until: &future}, "valid now"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := status(tc.w, at); !strings.Contains(got, tc.want) {
				t.Errorf("status = %q, want it to say %q", got, tc.want)
			}
		})
	}
}
