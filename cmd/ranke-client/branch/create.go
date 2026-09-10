// package: branch / cmd
// type:    entrypoint
// job:     `branch create` — bring a branch into being by contributing the claim that says so
// limits:  builds and sends one contribution; verification and the merge are the server's
//
// A branch is a name resolving to a closure, so it exists once a claim points at it. The
// creator's contributor travels along only where the archive holds none
// (-> client.ResolveContributor).
package branch

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/contributor"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// TypeBranchCreated records that a branch was established: `contribution/*` is the class for
// a claim about contributors' actions, its subtypes open where `R-C2TYPE` reserves only
// delete, expiry and branches to the Sequencer.
const TypeBranchCreated = "contribution/branch_created"

// createCmd sends the one contribution that establishes a branch.
func createCmd(inst *instance.Instance) *cobra.Command {
	var keySpec, pickSpec string
	var register bool
	c := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a branch by contributing the claim that records it",
		Long: "Contributes a claim recording that the branch was created, attributed to the\n" +
			"contributor the signing key resolves to: the claim the archive already holds\n" +
			"for that key, referenced across the branch boundary, or a freshly registered\n" +
			"one where the key is contributing for the first time. A key is registered\n" +
			"once and referenced from then on, a second claim over one pubkey being a\n" +
			"second identity rather than the same one twice.\n\n" +
			"A branch that already exists is reported as such and left alone, so this can be\n" +
			"run before every deployment without adding a second record of a creation that\n" +
			"happened once.\n\n" +
			"Needs C on $branches to add the entry, C on the branch to fill it, R on\n" +
			"$branches to see what exists, and R on $archive to resolve the key to the\n" +
			"contributor it already has there.",
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
			self, err := resolve(cmd, api, pair, pickSpec, register)
			if err != nil {
				return err
			}
			built, err := claims(name, self)
			if err != nil {
				return err
			}
			return send(cmd, api, inst.URL, name, self, built)
		},
	}
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"the contributor key to sign as: a path, file:path, env:VAR, stdin, or prompt")
	c.Flags().StringVar(&pickSpec, "contributor", "",
		"the id of the contributor claim to sign as, where the key carries more than one")
	c.Flags().BoolVar(&register, "register-identity", false,
		"register the signing key as a first-time contributor, without reading the archive "+
			"to check: for an account holding no R on $archive, and wrong for a key that "+
			"already has a contributor claim")
	return c
}

// resolve settles which contributor the record is attributed to, and reports it: a
// provisioning log wants to show whether the key was registered here or found already.
func resolve(cmd *cobra.Command, api *client.Client, pair ranke.Keypair, pickSpec string, register bool) (*client.Contributor, error) {
	if register {
		return client.RegisterContributor(pair)
	}
	pick, err := pickedContributor(pickSpec)
	if err != nil {
		return nil, err
	}
	// The record is dated now, so now is the date its contributor's key must admit
	// (`R-C4KEY`).
	self, err := api.ResolveContributor(cmd.Context(), pair, time.Now().UTC(), pick)
	if err != nil {
		return nil, err
	}
	out := cmd.OutOrStdout()
	if self.Registered {
		fmt.Fprintln(out, "contributor:   ", self.Claim.ID(), "(already registered,", self.Window, ")")
	} else {
		fmt.Fprintln(out, "contributor:   ", self.Claim.ID(), "(registering, first contribution)")
	}
	return self, nil
}

// pickedContributor reads the --contributor flag, which names one of several claims a key
// carries. `contributor list` is where an operator reads the ids.
func pickedContributor(spec string) (ranke.Id, error) {
	if spec == "" {
		return nil, nil
	}
	id, err := ranke.ParseId(spec)
	if err != nil {
		return nil, fmt.Errorf("--contributor %q: %w", spec, err)
	}
	return id, nil
}

// claims builds what the creation contributes: the record, preceded by the contributor claim
// where the key is being registered in the same breath.
func claims(name string, self *client.Contributor) ([]ranke.Claim, error) {
	// One past the contributor it cites (`V-HEIGHT`): a registration signs itself at 0, an
	// attested claim higher.
	record, err := ranke.NewClaim(TypeBranchCreated, self.As).
		WithInlineContent([]byte(name)).
		WithEncoding(ranke.EncodingText("plain")).
		WithHeight(self.Claim.Node().Height() + 1).
		Sign()
	if err != nil {
		return nil, fmt.Errorf("sign the creation claim: %w", err)
	}
	if self.Registered {
		return []ranke.Claim{record}, nil
	}
	return []ranke.Claim{self.Claim, record}, nil
}

// held reports whether the archive already carries the branch, asked because a creation
// claim records an event: re-running would date a second one to now. Needs R on $branches.
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

// send contributes the claims and reports the merge. Creating declares the branch brought
// into being, the server intersecting what it allows with what the stream asked for.
func send(cmd *cobra.Command, api *client.Client, url, name string, self *client.Contributor, built []ranke.Claim) error {
	// A registered contributor is cited where it stands, the archive scope reaching whichever
	// branch that is; a registration cites nothing outside itself.
	opts := []client.ContributeOption{client.Creating(), client.Referencing()}
	if self.Registered {
		opts = []client.ContributeOption{client.Creating(), client.Referencing(ranke.BranchArchive)}
	}
	res, err := api.Contribute(cmd.Context(), nil, name, built, opts...)
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
