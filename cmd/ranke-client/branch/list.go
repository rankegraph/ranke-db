// package: branch / cmd
// type:    entrypoint
// job:     `branch list` — the branch table, each branch with the head it resolves to
// limits:  reads and reports; the table is a claim the Sequencer mints (-> client.Branches)
package branch

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// listCmd reports the branch table.
func listCmd(inst *instance.Instance) *cobra.Command {
	var deep bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List the branches the archive holds",
		Long: "Reports every branch the branch table holds with the head it resolves to. A head\n" +
			"is a moving target, advanced by the next contribution that lands on it, and the\n" +
			"listing is answered from one archive snapshot, so the heads agree with each other.\n\n" +
			"--deep adds each head's height and when the branch last moved, which costs one\n" +
			"request per branch, so the set is no longer one snapshot.\n\n" +
			"Needs R on $branches, and R on each branch for --deep.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			api, err := inst.Connect()
			if err != nil {
				return err
			}
			return list(cmd.Context(), cmd.OutOrStdout(), api, inst.URL, deep)
		},
	}
	c.Flags().BoolVar(&deep, "deep", false,
		"also report each head's height and when the branch last moved, one request per branch")
	return c
}

// row is one branch as the listing prints it: the table's entry, with the detail --deep
// asks for.
type row struct {
	Name, Head, Detail string
}

// list reports the table, and the archive head above it, that being the id an $archive
// query or grant is held against and the only route that names it.
func list(ctx context.Context, out io.Writer, api *client.Client, url string, deep bool) error {
	branches, err := api.Branches(ctx)
	if err != nil {
		return fmt.Errorf("read the branches of %s: %w", url, err)
	}
	rows := make([]row, 0, len(branches))
	for _, b := range branches {
		r := row{Name: b.Name, Head: b.Head}
		if deep {
			info, err := api.BranchInfo(ctx, b.Name)
			if err != nil {
				return fmt.Errorf("read branch %q: %w", b.Name, err)
			}
			r.Detail = fmt.Sprintf("height %d, moved %s",
				info.Height, info.UpdatedAt.UTC().Format(time.RFC3339))
		}
		rows = append(rows, r)
	}
	var archive string
	if info, err := api.ArchiveInfo(ctx); err == nil {
		archive = info.Head
	}
	render(out, archive, rows)
	return nil
}

// render writes the listing, names aligned so a column of heads reads down the page. An
// archive with no branches says so rather than printing nothing at all.
func render(out io.Writer, archive string, rows []row) {
	if archive != "" {
		fmt.Fprintln(out, "archive:", archive)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "no branches")
		return
	}
	width := 0
	for _, r := range rows {
		if len(r.Name) > width {
			width = len(r.Name)
		}
	}
	for _, r := range rows {
		fmt.Fprintf(out, "  %-*s  %s\n", width, r.Name, r.Head)
		if r.Detail != "" {
			fmt.Fprintf(out, "  %-*s  %s\n", width, "", r.Detail)
		}
	}
}
