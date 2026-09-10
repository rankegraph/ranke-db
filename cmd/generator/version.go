// package: main / cmd
// type:    entrypoint
// job:     `generator version` — print this build's version
// limits:  CLI wiring; the value is internal/version's (-> internal/version)
package main

import (
	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/internal/version"
)

// versionCmd names this build, so a seeded fixture can be traced to what seeded it.
func versionCmd() *cobra.Command { return version.Command("generator") }
