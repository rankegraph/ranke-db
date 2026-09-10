// package: client / transport
// type:    logic
// job:     the contributor claim a signing key contributes under, and the registrations a scope
// holds, read back by key or in full
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

// Contributors returns the `contribution/contributor` claims scope holds, oldest first
// (`R-QSORT`), each carrying its pubkey inline — the keys that scope admits (`V-SIG`).
func (c *Client) Contributors(ctx context.Context, scope Scope) ([]ranke.Claim, error) {
	return c.byType(ctx, scope, ranke.NodeContributor)
}

// ContributorsFor returns the claims among those carrying pubkey, in the order they arrived. It
// reads the content because RQL filters fields alone, the set being one pubkey per contributor.
func (c *Client) ContributorsFor(ctx context.Context, scope Scope, pubkey []byte) ([]ranke.Claim, error) {
	if len(pubkey) == 0 {
		return nil, ErrNoPubkey
	}
	held, err := c.Contributors(ctx, scope)
	if err != nil {
		return nil, err
	}
	var carrying []ranke.Claim
	for _, claim := range held {
		content, err := claim.Node().GetInlineContent()
		if err != nil {
			continue
		}
		if bytes.Equal(content, pubkey) {
			carrying = append(carrying, claim)
		}
	}
	return carrying, nil
}

// Expiries returns the `contribution/expiry` claims scope holds, whose edge shortens a key's
// window (`R-DEXPIRY`). A contributor claim may carry such an edge too, so a window reads both.
func (c *Client) Expiries(ctx context.Context, scope Scope) ([]ranke.Claim, error) {
	return c.byType(ctx, scope, ranke.NodeExpiry)
}

// byType reads one scope's claims of one type, the shape the reads above take. `$universe`
// offers no head to walk and a read there requires one (`R-QHEAD`), so it is refused here.
func (c *Client) byType(ctx context.Context, scope Scope, typ string) ([]ranke.Claim, error) {
	switch scope {
	case "":
		return nil, ranke.ErrWireNoBranch
	case ScopeUniverse:
		return nil, ErrUniverseNeedsHead
	}
	return c.QueryClaims(ctx, ranke.Query{
		Select: ranke.Select{Branch: string(scope)},
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
