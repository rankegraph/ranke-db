// package: main / cmd
// type:    entrypoint
// job:     `ranke-db run` — assemble the stack from a config and serve every endpoint it mounts
// limits:  CLI wiring and the serve loop; the assembly is config's (-> config)
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-db/config"
)

// runCmd assembles the stack from a config and serves it.
func runCmd() *cobra.Command {
	var ageKey string
	var dev bool
	c := &cobra.Command{
		Use:   "run [flags] <configfile>|-",
		Short: "Assemble the adapter stack from a config and serve it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, cleanup, err := openConfig(args[0], ageKey)
			if err != nil {
				return err
			}
			defer cleanup()
			src, err := passphraseFrom(ageKey, os.Stdin)
			if err != nil {
				return err
			}
			app, err := config.Run(cmd.Context(), cfg, src, dev)
			if err != nil {
				return err
			}
			if err := requireServing(app); err != nil {
				return err
			}
			return serve(app)
		},
	}
	c.Flags().StringVar(&ageKey, "age-key", "", "age key source: prompt|stdin|env:VAR|file:path")
	c.Flags().BoolVar(&dev, "dev", false,
		"mount POST /dev/clock, steering the sequencer's clock instead of real time — requires sequencer.type \"dev\"; never for a real deployment")
	return c
}

// requireServing enforces what serving cannot do without: a signer, storage, a
// sequencer to reach the archive through, and an endpoint to reach them. Run assembles
// only what is configured; the policy lives here.
func requireServing(app *config.App) error {
	var missing []string
	if app.Signer == nil {
		missing = append(missing, "signer")
	}
	if app.Storage == nil {
		missing = append(missing, "storage")
	}
	if app.Sequencer == nil {
		// GetArchive is the sequencer's, and an archive is what every read opens, so a
		// stack without one answers nothing.
		missing = append(missing, "sequencer")
	}
	if len(app.Endpoints) == 0 {
		missing = append(missing, "endpoints")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config is missing required section(s) for serving: %v", missing)
	}
	return nil
}

// serve runs every endpoint the config mounted, concurrently, until the process is
// signalled. Each listens where its own section says: no flag can disagree.
func serve(app *config.App) error {
	logIdentity(app)
	logFounding(app)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	sctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errc := make(chan error, len(app.Endpoints))
	var wg sync.WaitGroup
	for _, ep := range app.Endpoints {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errc <- ep.Serve(sctx)
		}()
	}
	slog.Info("ranke-db serving", "endpoints", len(app.Endpoints))

	// The first endpoint to fail takes the process down: a stack serving less than
	// configured is not the stack the operator asked for.
	select {
	case err := <-errc:
		cancel()
		wg.Wait()
		return err
	case <-ctx.Done():
		slog.Info("ranke-db shutting down")
		cancel()
		wg.Wait()
		return nil
	}
}

// logIdentity reports which key the server attests merges under.
func logIdentity(app *config.App) {
	pub, err := app.Signer.Public(context.Background())
	if err != nil {
		slog.Warn("ranke-db: could not read signer identity", "err", err)
		return
	}
	id := fmt.Sprintf("%T", pub)
	if ed, ok := pub.(ed25519.PublicKey); ok {
		id = "ed25519:" + base64.RawStdEncoding.EncodeToString(ed)
	}
	slog.Info("ranke-db assembled", "signer", id)
}

// logFounding reports an archive this launch created. The ids are the operator's record:
// the bookmark is what reopens the archive if its seed is ever lost, and the contributor
// is the only trace of a founding key whose private half the server never held.
func logFounding(app *config.App) {
	if app.Founded == nil {
		return
	}
	slog.Info("ranke-db founded a new archive",
		"first_contributor", app.Founded.FirstContributor,
		"head", app.Founded.Head,
		"bookmark", app.Founded.Bookmark)
}
