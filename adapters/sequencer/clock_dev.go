// package: sequencer / coordination
// type:    adapter
// job:     a steerable clock for --dev — real time until told otherwise
// limits:  time source only; who may steer it is core's access decision (-> core)
package sequencer

import (
	"sync"
	"time"
)

// SteerableClock is a time source a caller can push forward — real time until the first
// Advance, then standing where it was put until told to move again. A story that wants
// its own dates steers to them; they run ahead of bt₀, which minted at boot from this
// same clock, so every later branch table still chains forward (R-C6MERGE, V-MONO).
type SteerableClock struct {
	mu  sync.Mutex
	set time.Time // zero until the first Advance; Now reads the epoch until then
}

// NewSteerableClock returns a clock reading real time until Advance is first called.
func NewSteerableClock() *SteerableClock { return &SteerableClock{} }

// Now is this clock's now func() time.Time source for sequencer.New.
func (c *SteerableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.set.IsZero() {
		return time.Now().UTC()
	}
	return c.set
}

// Advance moves the clock to at least t, returning its position afterward. t compares
// against the clock's own last position — zero before the first call, which is why that
// first call always lands exactly on t, whatever real time says. A t at or behind the
// current position is a no-op that just reports where the clock already is.
func (c *SteerableClock) Advance(t time.Time) time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t.After(c.set) {
		c.set = t
	}
	return c.set
}
