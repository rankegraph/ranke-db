// package: branch / cmd
// type:    entrypoint
// job:     `branch create` — bring a branch into being by contributing the claim its writer
// signs under
// limits:  builds and sends one contribution; the branch table and the merge are the server's
//
// A branch is a name resolving to a closure, so it exists once a claim points at it, and the
// contributor claim is the one every later claim on the branch needs anyway. It references nothing
// outside itself, so the branch is an independent graph from the start.
package branch

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/contributor"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// createCmd sends the one contribution that establishes a branch.
func createCmd(inst *instance.Instance) *cobra.Command {
	var keySpec string
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a branch by contributing the contributor claim it is written under",
		Long: "Contributes the creator's contributor claim, which brings the branch into being\n" +
			"and is what every later claim on it resolves through: a branch holding none\n" +
			"admits no writer (`V-SIG`). The creation itself needs no claim of its own — the\n" +
			"Sequencer records it in the branch-table revision that adds the entry, signed\n" +
			"under its own identity, and a client's account of the same event would only\n" +
			"restate it less credibly.\n\n" +
			"Each branch gets its own, so none of them references another and a tenant needs\n" +
			"rights over nothing but its own branches. Within a branch the claim is reused\n" +
			"rather than added again.\n\n" +
			"A branch that already exists is reported as such and left alone, so this can be\n" +
			"run before every deployment.\n\n" +
			"Needs C on $branches to add the entry, C on the branch to fill it, and R on\n" +
			"$branches to see what exists.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if err := ranke.ValidateBranchName(name); err != nil {
				return err
			}
			pair, err := contributor.Load(keySpec, cmd.InOrStdin())
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
			self, err := client.NewContributor(pair)
			if err != nil {
				return err
			}
			return send(cmd, api, inst.URL, name, self)
		},
	}
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"the contributor key to sign as: a path, file:path, env:VAR, stdin, or prompt")
	return c
}

// held reports whether the archive already carries the branch. Re-contributing the claim
// would be absorbed as the no-op it is — the Sequencer drops a head the branch already
// reaches — so this saves the round trip and answers plainly. Needs R on $branches.
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

// send contributes the claim and reports the merge. Creating declares the branch brought
// into being, the server intersecting what it allows with what the stream asked for, so a
// branch created by a typo is one the client asked for.
func send(cmd *cobra.Command, api *client.Client, url, name string, self ranke.Contributor) error {
	// Referencing nothing: the claim references nothing outside itself, which is what keeps the
	// branch independent of every other one and needs no read of any of them.
	res, err := api.Contribute(cmd.Context(), nil, name, []ranke.Claim{self},
		client.Creating(), client.Referencing())
	if err != nil {
		return fmt.Errorf("create %q on %s: %w", name, url, err)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "branch %q created\n", name)
	fmt.Fprintln(out, "  archive head:", res.Head)
	fmt.Fprintln(out, "  contributor: ", self.ID())
	return nil
}
