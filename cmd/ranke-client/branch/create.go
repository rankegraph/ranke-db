// package: branch / cmd
// type:    entrypoint
// job:     `branch create` — bring a branch into being by contributing the claim that says so
// limits:  builds and sends one contribution; verification and the merge are the server's
//
// A branch is a name resolving to a closure, so it exists once a claim points at it. The
// contribution carries two: the creator's own contributor claim, without which nothing in
// the branch could be signed, and a claim recording that the branch was created.
package branch

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/identity"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// TypeBranchCreated records that a branch was established. `contribution/*` is the class
// for a claim about contributors or their actions on the graph (foundation paper §Type
// Vocabulary), and subtype vocabulary is open; `R-C2TYPE` reserves only delete, expiry
// and branches to the Sequencer.
const TypeBranchCreated = "contribution/branch_created"

// createCmd sends the one contribution that establishes a branch.
func createCmd(inst *instance.Instance) *cobra.Command {
	var keySpec string
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a branch by contributing the claim that records it",
		Long: "Contributes the creator's contributor claim and a claim recording that the\n" +
			"branch was created. The contributor claim travels with it because a branch\n" +
			"holding none admits no writer: every claim's signature resolves through a\n" +
			"contributor its closure reaches.\n\n" +
			"A branch that already exists is reported as such and left alone, so this can be\n" +
			"run before every deployment without adding a second record of a creation that\n" +
			"happened once.\n\n" +
			"Needs C on $branches to add the entry, and C on the branch to fill it.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := ranke.ValidateBranchName(name); err != nil {
				return err
			}
			pair, err := identity.Load(keySpec, cmd.InOrStdin())
			if err != nil {
				return err
			}
			api, err := inst.Connect()
			if err != nil {
				return err
			}
			exists, err := held(cmd.Context(), api, inst.URL, name)
			if err != nil {
				return err
			}
			if exists {
				fmt.Fprintf(cmd.OutOrStdout(), "branch %q already exists, nothing to do\n", name)
				return nil
			}
			built, err := claims(name, pair)
			if err != nil {
				return err
			}
			return send(cmd, api, inst.URL, name, built)
		},
	}
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"the contributor key to sign as: a path, file:path, env:VAR, stdin, or prompt")
	return c
}

// claims builds the pair a creation carries: the contributor first, since the record
// below it references it, then the record itself.
func claims(name string, pair ranke.Keypair) ([]ranke.Claim, error) {
	// Epoch-dated, so one key yields one contributor id however often this runs — the
	// claim is an identity, not an event, and a fresh id each time would fork the
	// identity rather than reuse it.
	contributor, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(pair.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		Sign(pair.Private)
	if err != nil {
		return nil, fmt.Errorf("sign the contributor claim: %w", err)
	}
	self, err := contributor.AsContributor(context.Background(), nil, pair.Private)
	if err != nil {
		return nil, fmt.Errorf("read the contributor claim: %w", err)
	}
	record, err := ranke.NewClaim(TypeBranchCreated, self).
		WithInlineContent([]byte(name)).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(1).
		Sign(pair.Private)
	if err != nil {
		return nil, fmt.Errorf("sign the creation claim: %w", err)
	}
	return []ranke.Claim{contributor, record}, nil
}

// held reports whether the archive already carries the branch. Asked before contributing
// because a creation claim records an event: re-running would date a second one to now
// and advance the branch, where the operator meant "make sure this exists". Needs R on
// $branches, which is the read that pairs with the C this command uses.
func held(ctx context.Context, api *client.Client, url, name string) (bool, error) {
	branches, err := api.Branches(ctx)
	if err != nil {
		return false, fmt.Errorf("reach %s: %w", url, err)
	}
	for _, b := range branches {
		if b.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// send contributes the pair and reports what the merge produced. Creating declares the
// branch this brings into being: the server intersects what it allows with what the
// stream asked for, so a creation nobody declared is a creation that does not happen.
func send(cmd *cobra.Command, api *client.Client, url, name string, built []ranke.Claim) error {
	// The claims cite nothing outside themselves, a branch that does not exist yet
	// holding nothing to cite.
	res, err := api.Contribute(cmd.Context(), nil, name, built, client.Creating(), client.Referencing())
	if err != nil {
		return fmt.Errorf("create %q on %s: %w", name, url, err)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "branch %q created\n", name)
	fmt.Fprintln(out, "  archive head:", res.Head)
	for _, id := range res.Ids {
		fmt.Fprintln(out, "  claim:       ", id)
	}
	return nil
}
