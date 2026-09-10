// package: contributor / cmd
// type:    entrypoint
// job:     `contributor list` — the contributors the archive or one branch holds, with their
// key windows
// limits:  reads and reports; the answer is the server's (-> client.Contributors)
package contributor

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// listCmd reports the contributors of the archive, or of one branch.
func listCmd(inst *instance.Instance) *cobra.Command {
	var keySpec, branch string
	c := &cobra.Command{
		Use:   "list",
		Short: "List the contributors the archive holds, or the keys one branch admits",
		Long: "Reports each contribution/contributor claim: its id, the pubkey it carries, when\n" +
			"it was added, and the validity window its key holds — shortened where an expiry\n" +
			"was requested against it (`R-DEXPIRY`). One pubkey may be carried by several\n" +
			"claims — a branch holds its own, so a key writing to several has one in each —\n" +
			"and each is listed on its own.\n\n" +
			"--branch reads one branch instead, which is the keys that branch admits (`V-SIG`)\n" +
			"and the expiries binding there (`R-C3LIMIT`). It needs R on that branch alone,\n" +
			"where the whole archive needs R on $archive, a $-target no tenant holds.\n\n" +
			"--signing-key narrows the listing to one key's own.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if branch != "" {
				if err := ranke.ValidateBranchName(branch); err != nil {
					return err
				}
			}
			api, err := inst.Connect()
			if err != nil {
				return err
			}
			var only []byte
			if keySpec != "" {
				pair, err := Load(keySpec, cmd.InOrStdin())
				if err != nil {
					return err
				}
				only = pair.Pubkey
			}
			return list(cmd.Context(), cmd.OutOrStdout(), api, inst.URL, branch, only,
				time.Now().UTC())
		},
	}
	c.Flags().StringVar(&branch, "branch", "",
		"read one branch rather than the whole archive")
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"list only the contributors carrying this key's pubkey: a path, file:path, env:VAR, "+
			"stdin, or prompt")
	return c
}

// list reports the contributors of the scope read, narrowed to the pubkey only where one is
// given, with each window judged at at.
func list(
	ctx context.Context,
	out io.Writer,
	api *client.Client,
	url, branch string,
	only []byte,
	at time.Time,
) error {
	held, expiries, err := registrations(ctx, api, scope(branch))
	if err != nil {
		return fmt.Errorf("read the contributors of %s on %s: %w", scope(branch), url, err)
	}
	windows, err := client.ContributorWindows(held, expiries)
	if err != nil {
		return err
	}
	shown := 0
	for _, c := range held {
		pubkey, err := c.Node().GetInlineContent()
		if err != nil {
			continue
		}
		if only != nil && string(pubkey) != string(only) {
			continue
		}
		report(out, c, pubkey, windows[c.ID().String()], at)
		shown++
	}
	if shown == 0 {
		fmt.Fprintln(out, "no contributors in", scope(branch))
	}
	return nil
}

// registrations reads one scope's contributor claims and the expiries against them.
func registrations(ctx context.Context, api *client.Client, scope client.Scope) ([]ranke.Claim, []ranke.Claim, error) {
	held, err := api.Contributors(ctx, scope)
	if err != nil {
		return nil, nil, err
	}
	expiries, err := api.Expiries(ctx, scope)
	return held, expiries, err
}

// scope is the branch named, or the whole archive where none is — what a listing and its
// refusals say they answer for.
func scope(branch string) client.Scope {
	if branch == "" {
		return client.ScopeArchive
	}
	return client.Scope(branch)
}

// report writes one contributor's block.
func report(out io.Writer, c ranke.Claim, pubkey []byte, window client.ContributorWindow, at time.Time) {
	fmt.Fprintln(out, c.ID())
	fmt.Fprintln(out, "  pubkey  ", hex.EncodeToString(pubkey))
	fmt.Fprintln(out, "  created ", c.Node().CreatedAt().UTC().Format(time.RFC3339))
	fmt.Fprintln(out, "  window  ", window, "·", status(window, at))
}

// status judges a window at at, which is what decides whether a claim signed now under this
// contributor would verify (`R-C4KEY`).
func status(w client.ContributorWindow, at time.Time) string {
	switch {
	case w.Admits(at):
		return "valid now"
	case w.From != nil && at.Before(*w.From):
		return "not yet valid"
	default:
		return "lapsed"
	}
}
