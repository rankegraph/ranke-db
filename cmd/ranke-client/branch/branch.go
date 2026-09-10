// package: branch / cmd
// type:    entrypoint
// job:     the `branch` verbs — one file per subcommand, so `branch create` is create.go
// limits:  CLI wiring; the claims are the library's and the merge the server's (-> ranke-go)
package branch

import (
	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// Cmd groups the branch verbs.
func Cmd(inst *instance.Instance) *cobra.Command {
	c := &cobra.Command{
		Use:   "branch",
		Short: "Work with the archive's branches",
	}
	c.AddCommand(listCmd(inst), createCmd(inst))
	return c
}
