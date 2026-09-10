// package: contributor / cmd
// type:    entrypoint
// job:     `contributor add` — admit a key to a branch by contributing its contributor claim
// limits:  builds and sends one contribution; the branch table and the merge are the server's
//
// A branch admits the keys its own closure holds a contributor claim for (`V-SIG`), and
// `branch create` puts the creator's there. This is how the second writer arrives: a claim
// stating the new key, attributed to a contributor the branch already holds and signed under
// that one's key, so admission is the act of a writer already on the branch.
package contributor

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// addCmd sends the one contribution that admits a key to a branch.
func addCmd(inst *instance.Instance) *cobra.Command {
	var keySpec, pubSpec string
	c := &cobra.Command{
		Use:   "add <branch>",
		Short: "Admit a key to a branch by contributing the contributor claim that states it",
		Long: "Contributes a contribution/contributor claim carrying --pubkey, attributed to the\n" +
			"contributor claim --signing-key already holds on the branch and signed under that\n" +
			"key. From then on the admitted key signs its own claims there, each resolving\n" +
			"through the claim this adds (`V-SIG`).\n\n" +
			"The claim references that one contributor and nothing else, so the branch stays\n" +
			"independent of every other and a tenant needs rights over nothing but its own.\n\n" +
			"A key the branch already admits is reported as such and left alone, so this can be\n" +
			"run before every deployment.\n\n" +
			"Needs R on the branch to find the claim it is attributed to, and C on the branch\n" +
			"to add the new one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			if err := ranke.ValidateBranchName(branch); err != nil {
				return err
			}
			pubkey, err := Pubkey(pubSpec)
			if err != nil {
				return err
			}
			pair, err := Load(keySpec, cmd.InOrStdin())
			if err != nil {
				return err
			}
			api, err := inst.Connect()
			if err != nil {
				return err
			}
			return add(cmd.Context(), cmd.OutOrStdout(), api, inst.URL, branch, pubkey, pair)
		},
	}
	c.Flags().StringVar(&pubSpec, "pubkey", "",
		"the key to admit: the hex a listing prints, or a path to an Ed25519 public-key PEM")
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"the contributor key the new claim is attributed to and signed under: a path, "+
			"file:path, env:VAR, stdin, or prompt")
	if err := c.MarkFlagRequired("pubkey"); err != nil {
		panic(err) // the flag was just declared, so this cannot be a runtime condition
	}
	return c
}

// add builds the claim and contributes it, reporting what the branch now admits.
func add(
	ctx context.Context,
	out io.Writer,
	api *client.Client,
	url, branch string,
	pubkey []byte,
	pair ranke.Keypair,
) error {
	scope := client.Scope(branch)
	admitted, err := api.ContributorsFor(ctx, scope, pubkey)
	if err != nil {
		return fmt.Errorf("read the contributors of %q on %s: %w", branch, url, err)
	}
	if len(admitted) > 0 {
		fmt.Fprintf(out, "branch %q already admits this key, nothing to do\n", branch)
		return nil
	}
	mine, err := api.ContributorsFor(ctx, scope, pair.Pubkey)
	if err != nil {
		return fmt.Errorf("read the contributors of %q on %s: %w", branch, url, err)
	}
	if len(mine) == 0 {
		return fmt.Errorf("branch %q holds no contributor claim for this signing key, so nothing "+
			"there can be attributed to it: have a key the branch already admits run this, or "+
			"create the branch under this one", branch)
	}
	// The oldest claim for the key: with `branch create` it is the one the branch was founded
	// on, and any later one carries the same key.
	signing, err := mine[0].AsContributor(ctx, nil, pair.Private)
	if err != nil {
		return fmt.Errorf("read the contributor claim %s: %w", mine[0].ID(), err)
	}
	claim, err := admission(ctx, signing, pubkey)
	if err != nil {
		return err
	}
	res, err := api.Contribute(ctx, nil, branch, []ranke.Claim{claim})
	if err != nil {
		return fmt.Errorf("admit the key to %q on %s: %w", branch, url, err)
	}
	fmt.Fprintf(out, "branch %q admits the key\n", branch)
	fmt.Fprintln(out, "  contributor:   ", claim.ID())
	fmt.Fprintln(out, "  attributed to: ", signing.ID())
	fmt.Fprintln(out, "  archive head:  ", res.Head)
	return nil
}

// admission is the claim that states pubkey, attributed to signing and signed under its key,
// so what it registers is a key other than the one signing (`V-SIG`). The height is resolved
// against the claim it references, which carries its own (`V-HEIGHT`): a key admitted to the
// branch sits above the one that admitted it, and a reference the resolver was not given is
// reported rather than counted as 0.
func admission(ctx context.Context, signing ranke.Contributor, pubkey []byte) (ranke.Claim, error) {
	claim, err := ranke.NewClaim(ranke.NodeContributor, signing).
		WithInlineContent(pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeightResolver(ctx, ranke.HeightsFrom(signing)).
		Sign()
	if err != nil {
		return nil, fmt.Errorf("sign a contributor claim attributed to %s: %w", signing.ID(), err)
	}
	return claim, nil
}
