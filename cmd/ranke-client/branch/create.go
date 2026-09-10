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
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/cmd/ranke-client/identity"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
	"github.com/rankegraph/ranke-db/openapi/client"
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
			body, err := creation(name, pair)
			if err != nil {
				return err
			}
			return send(cmd, api, inst.URL, name, body)
		},
	}
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"the contributor key to sign as: a path, file:path, env:VAR, stdin, or prompt")
	return c
}

// creation builds the contribution stream from the claims below.
func creation(name string, pair ranke.Keypair) ([]byte, error) {
	built, err := claims(name, pair)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := ranke.NewWireWriter(&buf, ranke.WireConstraints{
		Branches:  []string{name},
		Creatable: []string{name},
	})
	for _, claim := range built {
		if err := w.WriteClaim(name, claim); err != nil {
			return nil, fmt.Errorf("write the contribution: %w", err)
		}
	}
	return buf.Bytes(), nil
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
func held(ctx context.Context, api *client.ClientWithResponses, url, name string) (bool, error) {
	answer, err := api.ListBranchesWithResponse(ctx)
	if err != nil {
		return false, fmt.Errorf("reach %s: %w", url, err)
	}
	if answer.StatusCode() != http.StatusOK {
		return false, fmt.Errorf("list branches: HTTP %d: %s", answer.StatusCode(), answer.Body)
	}
	if answer.JSON200 == nil {
		return false, nil
	}
	for _, b := range answer.JSON200.Branches {
		if b.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// send posts the stream and reports what the merge produced.
func send(cmd *cobra.Command, api *client.ClientWithResponses, url, name string, body []byte) error {
	answer, err := api.ContributeWithBodyWithResponse(cmd.Context(),
		"application/cbor-seq", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("reach %s: %w", url, err)
	}
	if answer.StatusCode() != http.StatusCreated {
		return fmt.Errorf("create %q: HTTP %d: %s", name, answer.StatusCode(), answer.Body)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "branch %q created\n", name)
	if answer.JSON201 != nil {
		fmt.Fprintln(out, "  archive head:", answer.JSON201.Head)
		for _, id := range answer.JSON201.Ids {
			fmt.Fprintln(out, "  claim:       ", id)
		}
	}
	return nil
}
