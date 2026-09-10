// package: main / cmd
// type:    entrypoint
// job:     `ranke-client version` — print this build's version
// limits:  CLI wiring; the value is internal/version's (-> internal/version)
package main

import (
	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/internal/version"
)

// versionCmd names this build — worth having in CI, where which client ran is part of
// what a log has to answer.
func versionCmd() *cobra.Command { return version.Command("ranke-client") }
