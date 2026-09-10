// package: main / cmd
// type:    entrypoint
// job:     the ranke-client binary — talk to a running instance over its REST contract
// limits:  CLI wiring only; the requests are the official client's (-> client)
//
// One binary per role: `ranke-db` operates an instance from its config, this one holds a
// contributor key and addresses a server already up. A contributor key signs claims into
// their ids where the server's identity attests the merge, so separate binaries keep two
// unlike keys apart in use.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/cmd/ranke-client/branch"
	"github.com/rankegraph/ranke-db/cmd/ranke-client/instance"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ranke-client:", err)
		os.Exit(1)
	}
}

// rootCmd builds the command tree. The credential flags are persistent because every verb
// addresses one instance. A subcommand lives in the file its name gives it: `whoami` in
// whoami.go, a group's verbs in a package of that name, so `branch create` is
// branch/create.go.
func rootCmd() *cobra.Command {
	var inst instance.Instance
	root := &cobra.Command{
		Use:           "ranke-client",
		Short:         "Read and contribute to a running ranke-db",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&inst.URL, "url", "http://localhost:8080",
		"base URL of the instance")
	root.PersistentFlags().StringVar(&inst.Token, "token", "",
		"Authorization: Bearer credential")
	root.PersistentFlags().StringVar(&inst.APIKey, "api-key", "",
		"X-API-Key credential")
	root.PersistentFlags().StringVar(&inst.Macaroon, "macaroon", "",
		"Authorization: Macaroon credential, base64 — the one credential carrying caveats")
	root.AddCommand(whoamiCmd(&inst), branch.Cmd(&inst), versionCmd())
	return root
}
