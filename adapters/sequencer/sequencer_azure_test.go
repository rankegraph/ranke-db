// Sequencer port test: the identity merges are signed as may be a Key Vault key,
// which signs under ES256 — the second scheme `V-SIGN` names.
package sequencer_test

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/adapters/signer/azure/azuretest"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-go"
)

// TestMergesSignedUnderES256 drives the whole chain with an Azure Key Vault identity:
// the Sequencer's contributor claim and every merge are signed through the vault,
// ranke-go frames that key as p256-pub and its envelopes as ES256, and verification
// reads the archive back. It skips without podman, as every real-counterpart test does.
func TestMergesSignedUnderES256(t *testing.T) {
	ctx := context.Background()
	cfg, teardown := azuretest.Setup(t)
	t.Cleanup(teardown)

	sig, err := signer.New(ctx, cfg)
	require.NoError(t, err)
	provision, ok := sig.(interface {
		PrepareKey(context.Context, string) (crypto.PublicKey, error)
	})
	require.True(t, ok, "the azure backend provisions the key it signs with")
	pub, err := provision.PrepareKey(ctx, "ranke-db-merges")
	require.NoError(t, err)
	require.IsType(t, &ecdsa.PublicKey{}, pub, "a Key Vault key signs under ES256")

	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": "a-list"}),
		ranke.NewMemoryUniverse(), sig, nil)
	require.NoError(t, err, "the contributor claim is signed through the vault")
	require.NotNil(t, seq.GetContributor(), "the sequencer signs as somebody")

	first := found(t, seq)
	require.NotEmpty(t, first.ID(), "founding names the first contributor")

	archive, err := seq.GetArchive(ctx)
	require.NoError(t, err)
	run, err := archive.Verify(ctx)
	require.NoError(t, err)
	run.Wait()
	require.NoError(t, run.Err())
	require.Empty(t, run.Failures(), "every claim verifies under the P-256 identity")
	require.Positive(t, run.Verified())
	t.Logf("▸ %d claim(s) verified, signed by %s", run.Verified(), signer.Identity(ctx, sig))
}
