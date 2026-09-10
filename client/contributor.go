// package: client / transport
// type:    logic
// job:     the contributor a signing key resolves to — the archive's registrations, the key
// window `R-DEXPIRY` fixes, and the choice between them
// limits:  reads and decides; contributing the result is contribute.go's and verifying it the
// server's (-> ranke-go verify_expiry)
//
// A pubkey enters the graph once, as one `contribution/contributor` claim, and is referenced
// from then on: a second claim over one key is a second identity, `V-ROOT` giving every claim
// exactly one contributor edge. Every app holding a contributor key needs this, so it lives
// here rather than in a command.
package client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rankegraph/ranke-go"
)

var (
	// ErrContributorUnresolved is a read the archive refused, so whether the key is
	// registered is unknown. Minting on that answer forks an identity, so the caller decides.
	ErrContributorUnresolved = errors.New("ranke/client: cannot tell whether this key is registered")
	// ErrContributorLapsed is a key whose every contributor claim is outside its validity window at
	// the date in hand, so nothing it signs would verify (`R-C4KEY`). It wants a rotation.
	ErrContributorLapsed = errors.New("ranke/client: this key is outside its validity window")
	// ErrContributorAmbiguous is a key with several contributor claims valid at once. There is no right
	// one to pick, so the caller names it.
	ErrContributorAmbiguous = errors.New("ranke/client: this key resolves to more than one contributor")
	// ErrNoSuchContributor is a named claim the archive does not hold under this key.
	ErrNoSuchContributor = errors.New("ranke/client: no such contributor claim for this key")
)

// Contributor is a contributor as an application holds it: the claim carrying its pubkey,
// bound to the key so it can sign, and where that claim stands.
type Contributor struct {
	// Claim is the `contribution/contributor` claim the pubkey lives in.
	Claim ranke.Claim
	// As attributes and signs the claims contributed under it.
	As ranke.Contributor
	// Registered reports Claim as one the archive holds, so a contribution references it
	// rather than carrying it.
	Registered bool
	// Window is the validity Claim states, shortened by any expiry against it.
	Window ContributorWindow
}

// Contributors returns the archive's `contribution/contributor` claims, oldest first
// (`R-QSORT`), each carrying its pubkey inline. Walks the closure, so needs R on `$archive`.
func (c *Client) Contributors(ctx context.Context) ([]ranke.Claim, error) {
	return c.byType(ctx, ranke.NodeContributor)
}

// Expiries returns the archive's `contribution/expiry` claims — the limiting claims whose
// edge shortens a contributor key's window (`R-DEXPIRY`). A successor contributor claim may
// carry such an edge too, so reading a window searches Contributors alongside these.
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

// ResolveContributor answers the contributor pair signs as for a claim dated at, taking the
// claim the archive already holds over minting a second.
//
// Which claim is no matter of taste: `R-C4KEY` admits only one whose key window contains at,
// so a lapsed claim is refused here rather than at the server's step 4. Where several are
// valid at once the archive is ambiguous and pick names the one to take. A key the archive
// does not know is a first-time contributor, whose claim the contribution must carry —
// everything it signs resolves through it (`V-SIG`).
func (c *Client) ResolveContributor(
	ctx context.Context, pair ranke.Keypair, at time.Time, pick ranke.Id,
) (*Contributor, error) {
	held, err := c.registered(ctx, pair.Pubkey)
	if err != nil {
		return nil, err
	}
	if len(held) == 0 {
		if pick != nil {
			return nil, fmt.Errorf("%w: %s", ErrNoSuchContributor, pick)
		}
		return RegisterContributor(pair)
	}
	expiries, err := c.Expiries(ctx)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: read the archive's expiries: %w", err)
	}
	windows, err := ContributorWindows(held, expiries)
	if err != nil {
		return nil, err
	}
	chosen, err := choose(held, windows, at, pick)
	if err != nil {
		return nil, err
	}
	as, err := chosen.AsContributor(ctx, nil, pair.Private)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: bind the contributor claim %s: %w", chosen.ID(), err)
	}
	return &Contributor{
		Claim:      chosen,
		As:         as,
		Registered: true,
		Window:     windows[chosen.ID().String()],
	}, nil
}

// RegisterContributor mints the contributor claim for a key the archive has never seen,
// which a contribution then carries. Epoch-dated, so one key yields one id however often a
// caller runs: the claim is an identity, not an event, and a fresh id would fork it.
func RegisterContributor(pair ranke.Keypair) (*Contributor, error) {
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(pair.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		Sign(pair.Private)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: sign the contributor claim: %w", err)
	}
	// The pubkey is inline, so binding it needs no Universe.
	as, err := claim.AsContributor(context.Background(), nil, pair.Private)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: read the contributor claim: %w", err)
	}
	return &Contributor{Claim: claim, As: as}, nil
}

// ContributorsFor keeps the claims whose inline content is pubkey, in the order they
// arrived. One shaped otherwise is passed over rather than failing the read.
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

// registered reads the contributor claims carrying pubkey, in the order the archive returns
// them, so a listing and a resolve agree on what a key holds.
func (c *Client) registered(ctx context.Context, pubkey []byte) ([]ranke.Claim, error) {
	found, err := c.Contributors(ctx)
	if errors.Is(err, ErrForbidden) {
		return nil, fmt.Errorf("%w: reading %s was refused, so an already-registered key "+
			"would be registered again — grant R on %s: %w",
			ErrContributorUnresolved, ranke.BranchArchive, ranke.BranchArchive, err)
	}
	if err != nil {
		return nil, fmt.Errorf("ranke/client: read the archive's contributors: %w", err)
	}
	return ContributorsFor(found, pubkey), nil
}

// choose settles which of the claims over one key is taken: the one pick names, else the one
// whose window admits at. It never falls back on an order, an archive holding two valid
// claims over one key having no answer a client could invent.
func choose(held []ranke.Claim, windows map[string]ContributorWindow, at time.Time, pick ranke.Id) (ranke.Claim, error) {
	if pick != nil {
		for _, c := range held {
			if c.ID().Equal(pick) {
				return c, nil
			}
		}
		return nil, fmt.Errorf("%w: %s", ErrNoSuchContributor, pick)
	}
	var valid []ranke.Claim
	for _, c := range held {
		if windows[c.ID().String()].Admits(at) {
			valid = append(valid, c)
		}
	}
	switch len(valid) {
	case 1:
		return valid[0], nil
	case 0:
		return nil, fmt.Errorf("%w at %s: %s", ErrContributorLapsed, rfc3339(at), spans(held, windows))
	default:
		return nil, fmt.Errorf("%w, each valid at %s: %s", ErrContributorAmbiguous, rfc3339(at), spans(valid, windows))
	}
}

// spans renders the claims with their windows, for a refusal a caller can act on.
func spans(held []ranke.Claim, windows map[string]ContributorWindow) string {
	out := make([]string, 0, len(held))
	for _, c := range held {
		out = append(out, c.ID().String()+" ("+windows[c.ID().String()].String()+")")
	}
	return strings.Join(out, ", ")
}

// stamp renders a date for a refusal.
func rfc3339(at time.Time) string { return at.UTC().Format(time.RFC3339) }
