// package: main / cmd
// type:    entrypoint
// job:     the generator binary — a client that seeds a running ranke-db over its REST API
// limits:  a client only: no config, no adapters, no archive of its own (-> cmd/ranke-db serves),
// and no transport of its own either (-> client)
//
// Seeding belongs to a client: a contributor is an application-held key (§5.7), so a
// fixture signs its own claims and the server attests only the merge. Filling a dev
// archive therefore goes through POST /contribute, the path everything else uses.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "generator:", err)
		os.Exit(1)
	}
}

// options are the flags every shape shares: where to write, as whom, with what key.
type options struct {
	branch   string
	branches int
	as       string
	token    string
	apiKey   string
	macaroon string
	wait     time.Duration
}

// connect points the official client at url with whatever credential was named. It is
// where the flags stop being strings: presenting more than one is refused here, as the
// endpoint routes on the scheme and could not resolve two.
func (o *options) connect(url string) (*client.Client, error) {
	var opts []client.Option
	if o.token != "" {
		opts = append(opts, client.WithToken(o.token))
	}
	if o.apiKey != "" {
		opts = append(opts, client.WithAPIKey(o.apiKey))
	}
	if o.macaroon != "" {
		opts = append(opts, client.WithMacaroon(o.macaroon))
	}
	return client.New(url, opts...)
}

// rootCmd builds the generator command tree: one subcommand per graph shape.
func rootCmd() *cobra.Command {
	var o options
	root := &cobra.Command{
		Use:           "generator",
		Short:         "Seed a running ranke-db with example graphs, over its REST API",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	f := root.PersistentFlags()
	f.StringVar(&o.branch, "branch", "main", "the primary branch; a shape spreading over several starts here")
	f.IntVar(&o.branches, "branches", 3, "how many branches to spread over — an archive with one exercises nothing about branches")
	f.StringVar(&o.as, "as", "dev", "contributor name; the same name always derives the same fixture identity")
	f.StringVar(&o.token, "token", "", "Authorization: Bearer credential")
	f.StringVar(&o.apiKey, "api-key", "", "X-API-Key credential")
	f.StringVar(&o.macaroon, "macaroon", "", "Authorization: Macaroon credential, base64")
	f.DurationVar(&o.wait, "wait", 0, "wait up to this long for the server to answer /health before writing")
	root.AddCommand(exampleCmd(&o), chainCmd(&o), releaseCmd(&o), versionCmd())
	return root
}

// exampleCmd writes the small hand-built graph: provenance you can read in full.
func exampleCmd(o *options) *cobra.Command {
	return &cobra.Command{
		Use:   "example <url>",
		Short: "Write the smallest graph with real provenance (4 claims, one contribution)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deliver(cmd, args[0], o, func(g *grower) (batches, error) {
				return g.example(branchesFor(o.branch, max(o.branches, 1)))
			})
		},
	}
}

// chainCmd grows an archive over many contributions — the shape that gets interesting.
func chainCmd(o *options) *cobra.Command {
	var contributions, per int
	c := &cobra.Command{
		Use:   "chain <url>",
		Short: "Grow an archive over many contributions, each citing what came before",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if contributions < 1 || per < 1 {
				return fmt.Errorf("chain: --contributions and --claims must both be at least 1")
			}
			return deliver(cmd, args[0], o, func(g *grower) (batches, error) {
				return g.chain(contributions, per, branchesFor(o.branch, max(o.branches, 1)))
			})
		},
	}
	c.Flags().IntVar(&contributions, "contributions", 20, "how many contributions to merge, one after another")
	c.Flags().IntVar(&per, "claims", 10, "claims per contribution")
	return c
}

// releaseCmd writes the release-process scenario: the shape the slides draw, built for real.
func releaseCmd(o *options) *cobra.Command {
	var releases int
	c := &cobra.Command{
		Use:   "release <url>",
		Short: "Write a release process: four signing identities, two packages, one artifact each release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return deliver(cmd, args[0], o, func(g *grower) (batches, error) {
				// One branch: a release process is one line, and the flag that spreads a
				// shape over several would say something about branches instead.
				return g.release(cmd.Context(), o.branch, releases)
			})
		},
	}
	c.Flags().IntVar(&releases, "releases", 3, "how many releases to run, each carrying both packages")
	return c
}

// progressEvery paces a long seed's reporting.
const progressEvery = 10

// deliver builds a shape's claims and merges each contribution in order — a claim may
// only cite what the archive already holds, so the batches go up one at a time.
func deliver(cmd *cobra.Command, url string, o *options, shape func(*grower) (batches, error)) error {
	ctx := cmd.Context()
	c, err := o.connect(url)
	if err != nil {
		return err
	}
	if o.wait > 0 {
		if err := c.WaitReady(ctx, o.wait); err != nil {
			return err
		}
	}

	g, err := newGrower(ctx, o.as)
	if err != nil {
		return err
	}
	bs, err := shape(g)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, ">> %s — contributing as %q\n", c.BaseURL(), o.as)
	total := 0
	perBranch := map[string]int{}
	seeded := map[string]bool{}
	for i, b := range bs {
		claims := b.claims
		// The contributor claim rides in each branch's first contribution: everything signed
		// references it, so a branch that has never seen it cannot resolve the closure.
		if !seeded[b.branch] {
			claims = append([]ranke.Claim{g.selfClaim}, claims...)
			seeded[b.branch] = true
		}
		// Steer the merge time to this batch's own story, before it lands — the first call
		// of the run included, so even the sequencer's bootstrap identity (minted at server
		// start, before any of this) is the last thing left dated off the real clock.
		if _, err := c.Dev().AdvanceClockPast(ctx, claims); err != nil {
			return fmt.Errorf("advance dev clock for contribution %d/%d: %w", i+1, len(bs), err)
		}
		// A generated graph cites only itself and the branch it joins, and every shape
		// but the first writes onto a branch that does not exist yet.
		res, err := c.Contribute(ctx, nil, b.branch, claims, client.Creating())
		if err != nil {
			return fmt.Errorf("contribution %d/%d onto %q: %w", i+1, len(bs), b.branch, err)
		}
		total += len(res.Ids)
		perBranch[b.branch] += len(res.Ids)
		if len(bs) > 1 && (i+1)%progressEvery == 0 {
			fmt.Fprintf(out, "   %d/%d contributions · %d claims\n", i+1, len(bs), total)
		}
	}

	fmt.Fprintf(out, ">> merged %d claim(s) in %d contribution(s) over %d branch(es)\n",
		total, len(bs), len(perBranch))
	for _, b := range branchesFor(o.branch, len(perBranch)) {
		if n, ok := perBranch[b]; ok {
			fmt.Fprintf(out, "   %-10s %d claims\n", b, n)
		}
	}
	return report(ctx, cmd, c, o.branch)
}

// report reads the branch back, so a seed that claims to have written shows it served.
func report(ctx context.Context, cmd *cobra.Command, c *client.Client, branch string) error {
	head, err := c.BranchHead(ctx, branch)
	if err != nil {
		return fmt.Errorf("read back %q: %w", branch, err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), ">> %s head %s\n", branch, head)
	return nil
}
