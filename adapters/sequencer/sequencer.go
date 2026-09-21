// package: sequencer / coordination
// type:    factory
// job:     the Sequencer port — build the backend a config names, over the bookmark list it advances
// limits:  wiring only; the head, the merges and the bookmarks are ranke-go's (-> ranke-go)
//
// 𝒰_hist is the Universe's own (Universe.Bookmarks), so there is no store to configure —
// only which list, as a seed or as the id of one entry.
package sequencer

import (
	"context"
	"fmt"
	"time"

	"github.com/rankegraph/ranke-go"
	"github.com/rankegraph/ranke-go/adapter/sequencer/concurrent"
	"github.com/rankegraph/ranke-go/adapter/sequencer/dev"

	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/config/scope"
)

// Sequencer is the sequencer port's product: ranke-go's Sequencer contract — immutable
// snapshots to read from, and merges that advance the head to write.
type Sequencer = ranke.Sequencer

// New builds the backend named by the section's "type". now is the time source claims are
// dated from; nil defaults to the wall clock, so a caller wanting a steerable one (--dev)
// supplies it instead.
//
// The archive is opened, never founded: an empty bookmark list comes back reporting
// InGenesis, leaving Found the composition root's call (-> config).
func New(ctx context.Context, cfg scope.Section, storage ranke.Universe, sig signer.Signer, now func() time.Time) (Sequencer, error) {
	if !cfg.HasValue("type") {
		return nil, fmt.Errorf("sequencer: missing type")
	}
	t, err := cfg.Get(ctx, "type")
	if err != nil {
		return nil, err
	}

	list, err := bookmarkList(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	self, err := contributor(ctx, storage, sig, now())
	if err != nil {
		return nil, err
	}
	clock := clockFunc(now)

	switch t {
	case "dev":
		return dev.NewSequencer(ctx, storage, list, self, clock)
	case "concurrent":
		return concurrent.NewSequencer(ctx, storage, list, self, clock)
	default:
		return nil, fmt.Errorf("sequencer: unknown type %q (want dev or concurrent)", t)
	}
}

// bookmarkList names the list in 𝒰_hist this instance advances, and exactly one key may:
// "seed" starts one at index 0, "bookmark" reopens a pruned one from a surviving entry
// (foundation paper §Backup). Any non-empty seed serves, `V-BMENV` saying SHOULD.
func bookmarkList(ctx context.Context, cfg scope.Section) (ranke.BookmarkLocator, error) {
	hasSeed, hasBookmark := cfg.HasValue("seed"), cfg.HasValue("bookmark")
	switch {
	case hasSeed && hasBookmark:
		return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: seed and bookmark are mutually exclusive")
	case !hasSeed && !hasBookmark:
		return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: one of seed or bookmark is required")
	case hasSeed:
		seed, err := cfg.Get(ctx, "seed")
		if err != nil {
			return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: seed: %w", err)
		}
		if seed == "" {
			return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: seed is empty")
		}
		return ranke.Seed([]byte(seed)), nil
	default:
		raw, err := cfg.Get(ctx, "bookmark")
		if err != nil {
			return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: bookmark: %w", err)
		}
		id, err := ranke.ParseId(raw)
		if err != nil {
			return ranke.BookmarkLocator{}, fmt.Errorf("sequencer: bookmark: %w", err)
		}
		return ranke.At(id), nil
	}
}

// contributor builds the identity merges are signed as, dated off the clock the merges
// take, so it precedes every merge it signs (`V-MONO`) and states a real instant.
func contributor(ctx context.Context, u ranke.Universe, sig signer.Signer, at time.Time) (ranke.Contributor, error) {
	key, err := signer.CryptoSigner(ctx, sig)
	if err != nil {
		return nil, fmt.Errorf("sequencer: %w", err)
	}
	encoded, err := ranke.EncodePublicKey(key.Public())
	if err != nil {
		return nil, fmt.Errorf("sequencer: encode public key: %w", err)
	}
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(encoded).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(at).
		Sign(key)
	if err != nil {
		return nil, fmt.Errorf("sequencer: sign contributor claim: %w", err)
	}
	return claim.AsContributor(ctx, u, key)
}

// clockFunc adapts a bare function to dev.Clock/concurrent.Clock — identically shaped
// (Tick() time.Time) in both, so one adapter serves either backend.
type clockFunc func() time.Time

// Tick reads the next timestamp.
func (f clockFunc) Tick() time.Time { return f() }
