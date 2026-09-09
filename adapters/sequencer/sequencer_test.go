// Sequencer port tests: the factory binds ranke-go's backends, and the identity it
// signs merges with comes through the signer port rather than from a local key.
package sequencer_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-go"
)

// signerConfig builds an inmemory signer descriptor over a throwaway Ed25519 key,
// in the PKCS#8 PEM form the adapter reads.
func signerConfig(t *testing.T) scope.Section {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return scope.Literal(map[string]string{"type": "inmemory", "key": string(key)})
}

// newSigner builds a signer through its own port, so this exercises the assembly
// the server performs rather than a hand-made double.
func newSigner(t *testing.T) signer.Signer {
	t.Helper()
	sig, err := signer.New(context.Background(), signerConfig(t))
	require.NoError(t, err)
	return sig
}

// found brings an archive into being under a throwaway key, returning the claim the
// Sequencer signed for it. Construction opens a bookmark list and writes nothing, so
// every case wanting a readable archive comes through here.
func found(t *testing.T, seq sequencer.Sequencer) ranke.Claim {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	encoded, err := ranke.EncodePublicKey(pub)
	require.NoError(t, err)
	first, err := seq.Found(context.Background(), encoded)
	require.NoError(t, err)
	return first
}

// TestNewBindsBackends: both backends build, and an unknown type is refused by name.
func TestNewBindsBackends(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []string{"dev", "concurrent"} {
		t.Run(kind, func(t *testing.T) {
			seq, err := sequencer.New(ctx,
				scope.Literal(map[string]string{"type": kind, "seed": "a-list"}),
				ranke.NewMemoryUniverse(), newSigner(t), nil)
			require.NoError(t, err)
			require.NotNil(t, seq)
			require.NotNil(t, seq.GetContributor(), "the sequencer signs as somebody")
		})
	}

	_, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "nope", "seed": "a-list"}),
		ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.ErrorContains(t, err, "unknown type")
}

// TestListIsNamedExactlyOnce: seed and bookmark carry different contracts — a list from
// index 0, against one reopened from a surviving entry — so naming both leaves it
// ambiguous which was meant, and naming neither leaves nothing to open.
func TestListIsNamedExactlyOnce(t *testing.T) {
	ctx := context.Background()

	_, err := sequencer.New(ctx, scope.Literal(map[string]string{"type": "dev"}),
		ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.ErrorContains(t, err, "one of seed or bookmark is required")

	_, err = sequencer.New(ctx, scope.Literal(map[string]string{
		"type": "dev", "seed": "a-list", "bookmark": "bciqbvsytb7eaithwgqgxwe5zxc6i3zuivbzbnq7ig2yyyztcjkkabga",
	}), ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.ErrorContains(t, err, "mutually exclusive")

	_, err = sequencer.New(ctx, scope.Literal(map[string]string{"type": "dev", "seed": ""}),
		ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.ErrorContains(t, err, "seed is empty")

	_, err = sequencer.New(ctx, scope.Literal(map[string]string{"type": "dev", "bookmark": "not-an-id"}),
		ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.ErrorContains(t, err, "bookmark")
}

// TestArchiveAwaitsFounding: construction opens a list and creates nothing, so a
// sequencer over an empty one reports InGenesis and refuses to be read from. Founding is
// what turns it into an archive, and a second attempt is refused.
func TestArchiveAwaitsFounding(t *testing.T) {
	ctx := context.Background()
	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": "a-list"}),
		ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.NoError(t, err)

	require.True(t, seq.InGenesis(), "nothing is founded yet")
	_, err = seq.GetArchive(ctx)
	require.ErrorIs(t, err, ranke.ErrSequencerGenesis, "and nothing can be read until it is")

	first := found(t, seq)
	require.NotEmpty(t, first.ID(), "founding names the first contributor")
	require.False(t, seq.InGenesis())

	archive, err := seq.GetArchive(ctx)
	require.NoError(t, err)
	branches, err := archive.GetBranches(ctx)
	require.NoError(t, err)
	require.Empty(t, branches, "a founded archive names no branches yet")
	require.NotEmpty(t, archive.Head(), "but it does have a head: the empty branch table")

	_, err = seq.Found(ctx, []byte("another key"))
	require.Error(t, err, "an archive is founded once")
}

// TestRestartReopensTheArchive is the regression that matters most here: this bug
// shipped in two releases. Construction used to mint k₀ and append it unconditionally,
// so a relaunch over a bookmark list that already held entries published a second and
// empty head above the real archive, leaving it unreachable while its claims sat intact
// in 𝒰. One universe and one seed across two builds is exactly that relaunch.
func TestRestartReopensTheArchive(t *testing.T) {
	ctx := context.Background()
	store := ranke.NewMemoryUniverse()
	cfg := scope.Literal(map[string]string{"type": "dev", "seed": "one-archive"})

	first, err := sequencer.New(ctx, cfg, store, newSigner(t), nil)
	require.NoError(t, err)
	found(t, first)
	before, err := first.GetArchive(ctx)
	require.NoError(t, err)

	again, err := sequencer.New(ctx, cfg, store, newSigner(t), nil)
	require.NoError(t, err)
	require.False(t, again.InGenesis(), "the list records an archive, so there is nothing to found")
	after, err := again.GetArchive(ctx)
	require.NoError(t, err)
	require.Equal(t, before.Head().String(), after.Head().String(),
		"a restart serves the head its bookmark list records")
}

// TestBookmarkReopensAPrunedList: the At arm is the way back in when index 0 is gone,
// since the record at any surviving entry carries the seed every bookmark in the list
// holds. Nothing here prunes — the point is that an id alone suffices, no seed needed.
func TestBookmarkReopensAPrunedList(t *testing.T) {
	ctx := context.Background()
	store := ranke.NewMemoryUniverse()

	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": "one-archive"}),
		store, newSigner(t), nil)
	require.NoError(t, err)
	found(t, seq)
	before, err := seq.GetArchive(ctx)
	require.NoError(t, err)

	again, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "bookmark": seq.BookmarkId().String()}),
		store, newSigner(t), nil)
	require.NoError(t, err)
	require.False(t, again.InGenesis())
	after, err := again.GetArchive(ctx)
	require.NoError(t, err)
	require.Equal(t, before.Head().String(), after.Head().String())
}

// TestContributorIdIsStableAcrossBuilds: one key yields one contributor id, so a
// restart reopens an archive as the same identity instead of minting a new one.
func TestContributorIdIsStableAcrossBuilds(t *testing.T) {
	ctx := context.Background()
	cfg := signerConfig(t)
	sequencerCfg := scope.Literal(map[string]string{"type": "dev", "seed": "a-list"})

	build := func() ranke.Id {
		sig, err := signer.New(ctx, cfg)
		require.NoError(t, err)
		seq, err := sequencer.New(ctx, sequencerCfg, ranke.NewMemoryUniverse(), sig, nil)
		require.NoError(t, err)
		return seq.GetContributor().ID()
	}

	require.Equal(t, build(), build())
}

// TestContributorAlwaysPinsToEpoch: the sequencer's own identity is minted once at
// boot, before any --dev caller can possibly steer the clock — the HTTP server isn't
// listening yet. It must precede whatever the earliest merge it signs turns out to be,
// so it stays epoch-pinned whether or not a clock was supplied, past-dated fixtures
// (a --dev story set in 2024, say) included; a value that tracked the clock instead
// would put this identity's created_at *after* the very first branch table it signs,
// which V-MONO forbids.
func TestContributorAlwaysPinsToEpoch(t *testing.T) {
	ctx := context.Background()
	cfg := scope.Literal(map[string]string{"type": "dev", "seed": "a-list"})

	seq, err := sequencer.New(ctx, cfg, ranke.NewMemoryUniverse(), newSigner(t), nil)
	require.NoError(t, err)
	require.True(t, seq.GetContributor().Node().CreatedAt().Equal(time.Unix(0, 0).UTC()),
		"nil now: want the identity pinned to the epoch")

	past := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	seq, err = sequencer.New(ctx, cfg, ranke.NewMemoryUniverse(), newSigner(t),
		func() time.Time { return past })
	require.NoError(t, err)
	require.True(t, seq.GetContributor().Node().CreatedAt().Equal(time.Unix(0, 0).UTC()),
		"a supplied now, even a --dev story's own past date: want the identity still pinned to the epoch")
}
