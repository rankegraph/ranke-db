// package: client / transport
// type:    test
// job:     every operation the contract declares is reachable from this package — the one thing a
// wrapper can silently stop short of, which sends a caller back to the generated client mid-task
// limits:  reachability and the plain answers; the reads and the write have their own files
package client_test

import (
	"context"
	"testing"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// TestEveryOperationIsReachable calls each of the contract's 21 operations through the
// package and expects an answer. A route missing a method here is a caller sent back to
// openapi/client halfway through a task, which is the seam this package removes.
func TestEveryOperationIsReachable(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	note := s.note(t, "reachable", storyTime)
	s.seed(t, c, note)

	run, err := c.StartVerification(ctx, client.VerificationConfig{Closure: testBranch})
	if err != nil {
		t.Fatalf("startVerification: %v", err)
	}

	for _, tc := range []struct {
		op   string
		call func() error
	}{
		{"health", func() error { _, err := c.Health(ctx); return err }},
		{"query", func() error {
			_, err := c.Query(ctx, ranke.Query{Select: ranke.Select{Branch: testBranch}})
			return err
		}},
		{"contribute", func() error {
			_, err := c.Contribute(ctx, s.Universe, testBranch, []ranke.Claim{s.Self})
			return err
		}},
		{"listBranches", func() error { _, err := c.Branches(ctx); return err }},
		{"getBranchHead", func() error { _, err := c.BranchHead(ctx, testBranch); return err }},
		{"getBranchInfo", func() error { _, err := c.BranchInfo(ctx, testBranch); return err }},
		{"getArchiveInfo", func() error { _, err := c.ArchiveInfo(ctx); return err }},
		{"getBranchClaim", func() error {
			_, err := c.GetClaim(ctx, client.Scope(testBranch), note.ID())
			return err
		}},
		{"getBranchClaimContent", func() error { return readContent(c, ctx, client.Scope(testBranch), note.ID()) }},
		{"getArchiveClaim", func() error { _, err := c.GetClaim(ctx, client.ScopeArchive, note.ID()); return err }},
		{"getArchiveClaimContent", func() error { return readContent(c, ctx, client.ScopeArchive, note.ID()) }},
		{"getClaim", func() error { _, err := c.GetClaim(ctx, client.ScopeUniverse, note.ID()); return err }},
		{"getClaimContent", func() error { return readContent(c, ctx, client.ScopeUniverse, note.ID()) }},
		{"advanceDevClock", func() error { _, err := c.Dev().AdvanceClock(ctx, storyTime); return err }},
		{"whoami", func() error { _, err := c.Whoami(ctx); return err }},
		{"listStorageLayers", func() error { _, err := c.Layers(ctx); return err }},
		{"listVerifications", func() error { _, err := c.Verifications(ctx); return err }},
		{"startVerification", func() error {
			_, err := c.StartVerification(ctx, client.VerificationConfig{Closure: testBranch})
			return err
		}},
		{"getVerification", func() error { _, err := c.Verification(ctx, run.Id); return err }},
		{"cancelVerification", func() error { _, err := c.CancelVerification(ctx, run.Id); return err }},
		{"deleteVerification", func() error { return c.DeleteVerification(ctx, run.Id) }},
	} {
		t.Run(tc.op, func(t *testing.T) {
			if err := tc.call(); err != nil {
				t.Fatalf("%s: %v", tc.op, err)
			}
		})
	}
}

// readContent fetches a claim's content and closes the body, for the routes whose
// answer is a stream.
func readContent(c *client.Client, ctx context.Context, scope client.Scope, id ranke.Id) error {
	content, err := c.GetContent(ctx, scope, id)
	if err != nil {
		return err
	}
	return content.Body.Close()
}

// TestTheStackDescribesItself pins the two system routes' answers rather than only
// their reachability: the account a credential resolves to, and the layers a read
// passes through.
func TestTheStackDescribesItself(t *testing.T) {
	ctx := context.Background()
	_, c := serve(t)

	who, err := c.Whoami(ctx)
	if err != nil {
		t.Fatalf("Whoami: %v", err)
	}
	if who.Account != testAccount || len(who.Grants) != len(everyRight) {
		t.Fatalf("subject = %+v, want %q holding %d grants", who, testAccount, len(everyRight))
	}

	if _, err := c.Layers(ctx); err != nil {
		t.Fatalf("Layers: %v", err)
	}
}

// TestVerificationRunsThroughItsLifecycle pins the five verification routes as one
// sequence: a run starts, is listed, is read, is cancelled with its report kept, and is
// deleted with nothing left behind.
func TestVerificationRunsThroughItsLifecycle(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)
	s.seed(t, c, s.note(t, "verified", storyTime))

	run, err := c.StartVerification(ctx, client.VerificationConfig{Closure: testBranch})
	if err != nil {
		t.Fatalf("StartVerification: %v", err)
	}
	if run.Id == "" || run.Head == "" {
		t.Fatalf("report = %+v, want the run identified and pinned to a head", run)
	}

	listed, err := c.Verifications(ctx)
	if err != nil {
		t.Fatalf("Verifications: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("the run that just started is not listed")
	}

	if _, err := c.Verification(ctx, run.Id); err != nil {
		t.Fatalf("Verification: %v", err)
	}
	if _, err := c.CancelVerification(ctx, run.Id); err != nil {
		t.Fatalf("CancelVerification: %v", err)
	}
	// Cancel keeps the report, so the run is still readable.
	if _, err := c.Verification(ctx, run.Id); err != nil {
		t.Fatalf("Verification after cancel: %v", err)
	}
	if err := c.DeleteVerification(ctx, run.Id); err != nil {
		t.Fatalf("DeleteVerification: %v", err)
	}
	if _, err := c.Verification(ctx, run.Id); err == nil {
		t.Fatal("a deleted run is still readable")
	}
}
