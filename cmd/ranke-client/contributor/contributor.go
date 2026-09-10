// package: contributor / cmd
// type:    entrypoint
// job:     the `contributor` verbs — one file per subcommand, so `contributor list` is list.go
// limits:  CLI wiring; resolving a signing key is load.go's, a pubkey pubkey.go's, and the
// merge the server's
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
			"claims are signed under. A branch admits the keys its own closure holds such a\n" +
			"claim for, and within a branch the claim is referenced rather than added again.",
	}
	c.AddCommand(listCmd(inst), addCmd(inst))
	return c
}
