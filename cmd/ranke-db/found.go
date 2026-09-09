// package: main / cmd
// type:    entrypoint
// job:     `ranke-db found` — bring a config's archive into being, once, then exit
// limits:  CLI wiring; the founding is config's (-> config)
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/config"
)

// foundCmd brings an archive into being, once, and exits. It is the path for a config
// that names no founder; one that does founds on its own at launch.
func foundCmd() *cobra.Command {
	var ageKey string
	c := &cobra.Command{
		Use:   "found [flags] <configfile>|- <pubkeyfile>",
		Short: "Found this config's archive under a first contributor's public key, then exit",
		Long: "Creates the archive a config points at: the sequencer's initial claim, the first\n" +
			"contributor carrying the given PEM public key, the empty branch table k₀ and its\n" +
			"bookmark. It never serves. An archive that already exists is reported as such and\n" +
			"left alone, so provisioning may call this ahead of every launch.\n\n" +
			"The private half of the founding key never reaches the server: hand over the public\n" +
			"half and keep the rest with the application that contributes under it.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			pubkey, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("found: read the public key: %w", err)
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
			founding, err := config.Found(cmd.Context(), cfg, src, pubkey)
			if errors.Is(err, config.ErrAlreadyFounded) {
				fmt.Fprintln(cmd.OutOrStdout(), "already founded, nothing to do")
				return nil
			}
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "archive founded")
			fmt.Fprintln(out, "  first contributor:", founding.FirstContributor)
			fmt.Fprintln(out, "  head:             ", founding.Head)
			fmt.Fprintln(out, "  bookmark:         ", founding.Bookmark)
			return nil
		},
	}
	c.Flags().StringVar(&ageKey, "age-key", "", "age key source: prompt|stdin|env:VAR|file:path")
	return c
}
