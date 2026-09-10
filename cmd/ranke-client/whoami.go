// package: main / cmd
// type:    entrypoint
// job:     `ranke-client whoami` — report what this credential may do
// limits:  CLI wiring; the answer is the server's (-> GET /system/whoami)
package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

// whoamiCmd asks the instance what the credential resolved to. It needs no grant, so it
// answers for an account holding nothing, which is the case worth asking about.
func whoamiCmd(inst *instance.Instance) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Report the account this credential resolves to, and what it may do",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			api, err := inst.Connect()
			if err != nil {
				return err
			}
			subject, err := api.Whoami(cmd.Context())
			if err != nil {
				return fmt.Errorf("reach %s: %w", inst.URL, err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "account:", subject.Account)
			for _, g := range subject.Grants {
				fmt.Fprintln(out, "  grant: ", g)
			}
			for _, c := range subject.Caveats {
				fmt.Fprintln(out, "  caveat:", c)
			}
			return nil
		},
	}
}
