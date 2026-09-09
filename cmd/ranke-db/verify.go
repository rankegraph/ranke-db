// package: main / cmd
// type:    entrypoint
// job:     `ranke-db verify` — check a config to a chosen depth, without serving
// limits:  CLI wiring; the checks themselves are config's (-> config)
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/config"
)

// verifyCmd checks a config to the chosen depth and reports, without serving.
func verifyCmd() *cobra.Command {
	var ageKey, levelName string
	c := &cobra.Command{
		Use:   "verify [flags] <configfile>|-",
		Short: "Check a config to a chosen depth (syntax|resolve)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			level, err := parseLevel(levelName)
			if err != nil {
				return err
			}
			cfg, cleanup, err := openConfig(args[0], ageKey)
			if err != nil {
				return err
			}
			defer cleanup()
			src, err := passphraseFrom(ageKey, os.Stdin)
			if err != nil {
				return err
			}
			if err := config.Verify(cmd.Context(), cfg, src, level); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "config ok")
			return nil
		},
	}
	c.Flags().StringVar(&ageKey, "age-key", "", "age key source: prompt|stdin|env:VAR|file:path")
	c.Flags().StringVar(&levelName, "level", "syntax", "verify depth: syntax|resolve|connect")
	return c
}

// parseLevel maps the --level flag to a config.Level.
func parseLevel(s string) (config.Level, error) {
	switch s {
	case "syntax", "":
		return config.LevelSyntax, nil
	case "resolve":
		return config.LevelResolve, nil
	case "connect":
		return config.LevelConnect, nil
	default:
		return 0, fmt.Errorf("unknown --level %q (want syntax|resolve|connect)", s)
	}
}
