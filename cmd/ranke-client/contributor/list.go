// package: contributor / cmd
// type:    entrypoint
// job:     `contributor list` — every contributor the archive holds, with its key window
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

// listCmd reports the archive's contributors.
func listCmd(inst *instance.Instance) *cobra.Command {
	var keySpec string
	c := &cobra.Command{
		Use:   "list",
		Short: "List the contributors the archive holds",
		Long: "Reports each contribution/contributor claim: its id, the pubkey it carries, when\n" +
			"it was added, and the validity window its key holds — shortened where an expiry\n" +
			"was requested against it (`R-DEXPIRY`). One pubkey may be carried by several\n" +
			"claims — a branch holds its own, so a key writing to several has one in each —\n" +
			"and each is listed on its own.\n\n" +
			"Needs R on $archive. --signing-key narrows the listing to one key's own.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			return list(cmd.Context(), cmd.OutOrStdout(), api, inst.URL, only, time.Now().UTC())
		},
	}
	c.Flags().StringVar(&keySpec, "signing-key", "",
		"list only the contributors carrying this key's pubkey: a path, file:path, env:VAR, "+
			"stdin, or prompt")
	return c
}

// list reports the contributors, narrowed to the pubkey only where one is given, with each
// window judged at at.
func list(ctx context.Context, out io.Writer, api *client.Client, url string, only []byte, at time.Time) error {
	held, err := api.Contributors(ctx)
	if err != nil {
		return fmt.Errorf("read the contributors of %s: %w", url, err)
	}
	expiries, err := api.Expiries(ctx)
	if err != nil {
		return fmt.Errorf("read the expiries of %s: %w", url, err)
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
		fmt.Fprintln(out, "no contributors")
	}
	return nil
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
