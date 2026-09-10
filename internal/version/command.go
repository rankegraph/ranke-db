// package: version / build
// type:    logic
// job:     Command — the `version` verb every binary carries, so one build names itself
// the same way whichever you ran
// limits:  prints what version.String resolved (-> version.go)
package version

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Command builds the `version` subcommand for binary. Shared so the three agree on the
// wording: a release cut from one tag should not describe itself two ways.
func Command(binary string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print this build's version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), binary, String())
			return err
		},
	}
}
