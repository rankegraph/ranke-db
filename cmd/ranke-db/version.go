// package: main / cmd
// type:    entrypoint
// job:     `ranke-db version` — print this build's version
// limits:  CLI wiring; the value is internal/version's (-> internal/version)
package main

import (
	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/internal/version"
)

// versionCmd names this build, the same string /health and / report.
func versionCmd() *cobra.Command { return version.Command("ranke-db") }
