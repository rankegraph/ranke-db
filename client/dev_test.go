// package: client / transport
// type:    test
// job:     POST /dev/clock — that it steers a dev stack's clock, and that a production stack's
// refusal comes back as absence rather than as failure
// limits:  the dev route alone
package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// TestAdvanceClockSteersADevStack pins the route doing its job: the clock moves to the
// instant asked for, and never backwards.
func TestAdvanceClockSteersADevStack(t *testing.T) {
	ctx := context.Background()
	_, c := serve(t)

	at := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	clock, err := c.Dev().AdvanceClock(ctx, at)
	if err != nil {
		t.Fatalf("AdvanceClock: %v", err)
	}
	if !clock.Available {
		t.Fatal("a dev stack reported no clock")
	}
	if clock.Time.Before(at) {
		t.Fatalf("clock = %s, want it at or past %s", clock.Time, at)
	}

	// An instant already passed is accepted and changes nothing: a merge's witnessed
	// time regressing would break every guarantee built on it.
	back, err := c.Dev().AdvanceClock(ctx, at.Add(-time.Hour))
	if err != nil {
		t.Fatalf("AdvanceClock backwards: %v", err)
	}
	if back.Time.Before(clock.Time) {
		t.Fatalf("clock went back to %s from %s", back.Time, clock.Time)
	}
}

// TestAdvanceClockOnAProductionStack pins why this is not an error. A stack that mounts
// no dev routes answers 501, which comes back as Available false — so one binary runs
// against a dev and a production stack without branching on the deployment.
func TestAdvanceClockOnAProductionStack(t *testing.T) {
	_, c := serve(t, withoutDevClock())

	clock, err := c.Dev().AdvanceClock(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("AdvanceClock against a production stack = %v, want no error", err)
	}
	if clock.Available {
		t.Fatal("a stack mounting no dev routes reported a clock")
	}
}

// TestAdvanceClockPastFollowsTheStory pins what a dev contribution needs: the sequencer
// stamps its merge from the clock, and a merge dated before a claim it absorbs breaks
// `V-MONO`, which a fixed offset from wall-clock now cannot prevent.
func TestAdvanceClockPastFollowsTheStory(t *testing.T) {
	ctx := context.Background()
	s, c := serve(t)

	early := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	late := time.Date(2026, 3, 1, 17, 0, 0, 0, time.UTC)
	claims := []ranke.Claim{s.note(t, "morning", early), s.note(t, "evening", late)}

	if latest := client.MaxCreatedAt(claims); !latest.Equal(late) {
		t.Fatalf("MaxCreatedAt = %s, want %s", latest, late)
	}

	clock, err := c.Dev().AdvanceClockPast(ctx, claims)
	if err != nil {
		t.Fatalf("AdvanceClockPast: %v", err)
	}
	if clock.Time.Before(late) {
		t.Fatalf("clock = %s, want it at or past the batch's own latest %s", clock.Time, late)
	}

	// No claims is no instant to steer to, so the clock is left where it stands.
	if got, err := c.Dev().AdvanceClockPast(ctx, nil); err != nil || got.Available {
		t.Fatalf("AdvanceClockPast(nil) = %+v, %v, want an untouched clock", got, err)
	}
}
