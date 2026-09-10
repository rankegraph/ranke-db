// package: client / transport
// type:    logic
// job:     the contributor claim a signing key contributes under, and the archive's
// registrations for reading back
// limits:  builds and reads; contributing it is contribute.go's and verifying it the server's
//
// A branch holds its own contributor claim for a key, dated when it was added — the moment
// the archive witnessed (paper 01 §Nodes) — so no branch depends on another's.
package client

import (
	"bytes"
	"context"
	"fmt"

	"github.com/rankegraph/ranke-go"
)

// Contributors returns the archive's `contribution/contributor` claims, oldest first
// (`R-QSORT`), each carrying its pubkey inline. Walks the closure, so needs R on `$archive`.
func (c *Client) Contributors(ctx context.Context) ([]ranke.Claim, error) {
	return c.byType(ctx, ranke.NodeContributor)
}

// Expiries returns the archive's `contribution/expiry` claims, whose edge shortens a key's
// window (`R-DEXPIRY`). A contributor claim may carry such an edge too, so a window reads both.
func (c *Client) Expiries(ctx context.Context) ([]ranke.Claim, error) {
	return c.byType(ctx, string(ranke.NodeClassContribution)+"/"+string(ranke.NodeSubtypeExpiry))
}

// byType reads the archive's claims of one type, the shape both reads above take.
func (c *Client) byType(ctx context.Context, typ string) ([]ranke.Claim, error) {
	return c.QueryClaims(ctx, ranke.Query{
		Select: ranke.Select{Branch: ranke.BranchArchive},
		Where:  &ranke.Where{Field: "type", Test: &ranke.Comparison{Eq: typ}},
	})
}

// NewContributor builds the contributor claim pair signs under, bound to the key so it signs
// at once. A contribution carries it, every signature resolving through a contributor its own
// closure reaches (`V-SIG`).
//
// Dated now, `created_at` being the moment the claim was added and nothing else (paper 01
// §Nodes) — so a caller whose clock runs ahead of the server's is refused by the ceiling
// `R-C2DATE` sets at the base time.
func NewContributor(pair ranke.Keypair) (ranke.Contributor, error) {
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(pair.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		Sign(pair.Private)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: sign the contributor claim: %w", err)
	}
	// The pubkey is inline, so binding it needs no Universe.
	self, err := claim.AsContributor(context.Background(), nil, pair.Private)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: read the contributor claim: %w", err)
	}
	return self, nil
}

// ContributorsFor keeps the claims whose inline content is pubkey, in the order they arrived.
// It reads the content because RQL filters fields alone, and the set is small — a pubkey per
// contributor.
func ContributorsFor(claims []ranke.Claim, pubkey []byte) []ranke.Claim {
	var held []ranke.Claim
	for _, c := range claims {
		content, err := c.Node().GetInlineContent()
		if err != nil {
			continue
		}
		if bytes.Equal(content, pubkey) {
			held = append(held, c)
		}
	}
	return held
}
