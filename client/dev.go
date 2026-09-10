// package: client / transport
// type:    adapter
// job:     POST /dev/clock — the dev sequencer's clock, reachable only through Client.Dev so a
// production consumer never meets it on the client's own surface
// limits:  the route alone; whether it exists is the stack's launch flag, and this reports its
// absence rather than failing on it
package client

import (
	"context"
	"errors"
	"time"

	"github.com/rankegraph/ranke-go"

	api "github.com/rankegraph/ranke-db/openapi/client"
)

// DevClient is the development-only surface. It is reached through Client.Dev, which
// keeps these routes out of the way of a consumer that has no business with them: a
// real deployment mounts none of them.
type DevClient struct{ c *Client }

// Dev returns the development-only surface.
func (c *Client) Dev() *DevClient { return &DevClient{c: c} }

// Clock is where the dev sequencer's clock stands. Available is false against a stack
// that mounts no dev routes, which is every production one.
type Clock struct {
	Time      time.Time
	Available bool
}

// AdvanceClock moves the dev sequencer's clock to at least at, so a client whose story
// has its own schedule can make the archive's recorded times follow it.
//
// The clock only moves forward: an instant already passed is accepted and changes
// nothing. A stack without the dev routes answers 501, which comes back as Available
// false rather than as an error — a caller running against both kinds of deployment
// should not have to branch on which one it reached.
func (d *DevClient) AdvanceClock(ctx context.Context, at time.Time) (Clock, error) {
	res, err := d.c.api.AdvanceDevClockWithResponse(ctx, api.DevClockAdvance{Time: at.UTC()})
	if err != nil {
		return Clock{}, err
	}
	if res.JSON200 == nil {
		err := refusal(res.StatusCode(), res.Body)
		if errors.Is(err, ErrUnimplemented) || errors.Is(err, ErrNotFound) {
			return Clock{}, nil
		}
		return Clock{}, err
	}
	return Clock{Time: res.JSON200.Time, Available: true}, nil
}

// AdvanceClockPast moves the clock to the latest created_at the claims carry, which is
// what a dev contribution needs: the sequencer stamps its merge from the clock, and a
// merge dated before a claim it absorbs would break `V-MONO`. A fixed offset from
// wall-clock now cannot know where a batch's own story ends.
func (d *DevClient) AdvanceClockPast(ctx context.Context, claims []ranke.Claim) (Clock, error) {
	at := MaxCreatedAt(claims)
	if at.IsZero() {
		return Clock{}, nil
	}
	return d.AdvanceClock(ctx, at)
}

// MaxCreatedAt is the latest created_at among claims, zero when there are none.
func MaxCreatedAt(claims []ranke.Claim) time.Time {
	var latest time.Time
	for _, c := range claims {
		if c == nil {
			continue
		}
		if at := c.Node().CreatedAt(); at.After(latest) {
			latest = at
		}
	}
	return latest
}
