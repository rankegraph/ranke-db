// package: client / transport
// type:    adapter
// job:     POST /contribute — a claim set and the external content it names, sent as one
// contribution stream, with the content gathered from the Universe rather than left to the caller
// limits:  builds and posts the stream; framing it is the library's (-> ranke-go codec_wire), and
// admitting it is the server's Sequencer
package client

import (
	"bytes"
	"context"
	"errors"

	"github.com/rankegraph/ranke-go"
)

// ContributeOption declares what a contribution asks beyond appending to a branch that
// exists. A declaration only narrows what the server allows, so asking still needs the
// right to match.
type ContributeOption func(*ranke.WireConstraints)

// Creating declares that the branches named may be brought into being. Without it a
// contribution to a branch the archive lacks is refused however the account is granted,
// so a branch created by a typo is one the client asked for.
func Creating() ContributeOption {
	return func(c *ranke.WireConstraints) { c.Creatable = append(c.Creatable, c.Branches...) }
}

// Referencing replaces the scopes the claims may reference, which default to the branch being
// written. The server narrows the declaration to what the grants reach.
func Referencing(scopes ...string) ContributeOption {
	return func(c *ranke.WireConstraints) { c.Referencable = scopes }
}

// Contribute merges claims into branch atomically, with the content they address, and
// returns the new branch-table head with the ids that landed. Content addressing makes
// it idempotent: re-contributing yields the same ids.
//
// It takes a Universe so the blobs cannot be forgotten — a claim carries only its
// content_hash, and dedup reads that hash rather than fetching it. Claims naming no
// external content need none, and nil is then the honest answer.
func (c *Client) Contribute(
	ctx context.Context,
	u ranke.Universe,
	branch string,
	claims []ranke.Claim,
	opts ...ContributeOption,
) (*ContributionResult, error) {
	if branch == "" {
		return nil, ranke.ErrWireNoBranch
	}
	if len(claims) == 0 {
		return &ContributionResult{}, nil
	}
	refs, err := ExternalContent(claims)
	if err != nil {
		return nil, err
	}
	blobs, err := fetchContent(ctx, u, refs)
	if err != nil {
		return nil, err
	}

	cons := ranke.WireConstraints{Branches: []string{branch}, Referencable: []string{branch}}
	for _, o := range opts {
		o(&cons)
	}
	// The constraints head the stream, so C is settled before the payload is read.
	var buf bytes.Buffer
	w := ranke.NewWireWriter(&buf, cons)
	for _, claim := range claims {
		if err := w.WriteClaim(branch, claim); err != nil {
			return nil, err
		}
	}
	for _, blob := range blobs {
		if err := w.WriteContent(blob); err != nil {
			return nil, err
		}
	}

	res, err := c.api.ContributeWithBodyWithResponse(ctx, ranke.WireMediaType, bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, err
	}
	if res.JSON201 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON201, nil
}

// ExternalContent lists each distinct blob the claims address, edges included, in order
// of first appearance so a stream reproduces.
func ExternalContent(claims []ranke.Claim) ([]ranke.ContentRef, error) {
	var refs []ranke.ContentRef
	seen := map[string]bool{}
	add := func(hash ranke.Id, size uint64) {
		if hash == nil || seen[hash.String()] {
			return
		}
		seen[hash.String()] = true
		refs = append(refs, ranke.ContentRef{Hash: hash, ContentSize: size})
	}
	for _, claim := range claims {
		if claim == nil {
			return nil, ErrNilClaim
		}
		n := claim.Node()
		add(n.GetContentHash(), n.GetContentSize())
		for _, e := range claim.Edges() {
			add(e.GetContentHash(), e.GetContentSize())
		}
	}
	return refs, nil
}

// fetchContent reads each blob from the Universe the claims were built against.
func fetchContent(ctx context.Context, u ranke.Universe, refs []ranke.ContentRef) ([]ranke.ContentBlob, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if u == nil {
		return nil, ErrNoUniverse
	}
	data, err := u.GetContents(ctx, refs)
	if err != nil {
		// The failure this exists to catch, under a name that says so — cause kept, an
		// unreachable backend reporting the same absence.
		if errors.Is(err, ranke.ErrNotFound) {
			return nil, errors.Join(ErrContentMissing, err)
		}
		return nil, err
	}
	blobs := make([]ranke.ContentBlob, 0, len(refs))
	for i, ref := range refs {
		if i >= len(data) || data[i] == nil {
			return nil, ranke.WithDetail(ErrContentMissing, ref.Hash.String())
		}
		blobs = append(blobs, ranke.ContentBlob{Hash: ref.Hash, Content: data[i]})
	}
	return blobs, nil
}
