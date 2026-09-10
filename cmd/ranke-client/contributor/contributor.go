// package: contributor / cmd
// type:    entrypoint
// job:     the `contributor` verbs — one file per subcommand, so `contributor list` is list.go
// limits:  CLI wiring; resolving a key is resolve.go's and the claims the server's
package contributor

import (
	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// Cmd groups the contributor verbs.
func Cmd(inst *instance.Instance) *cobra.Command {
	c := &cobra.Command{
		Use:   "contributor",
		Short: "Work with the archive's contributors",
		Long: "A contributor is the actor a claim is attributed to, carrying the pubkey its\n" +
			"claims are signed under. A pubkey enters the graph once, as one\n" +
			"contribution/contributor claim, and is referenced from then on.",
	}
	c.AddCommand(listCmd(inst))
	return c
}
