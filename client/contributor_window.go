// package: client / transport
// type:    logic
// job:     `R-DEXPIRY`'s key window — the bounds a contributor claim states, shortened by any
// expiry edge naming it
// limits:  reads the bounds and answers whether a date falls inside; the server verifies
// (-> ranke-go verify_expiry)
package client

import (
	"fmt"
	"time"

	"github.com/rankegraph/ranke-go"
)

// Window is a contributor key's validity: the closed span between its bounds, each standing
// on its own, an absent bound leaving that end open (`R-DEXPIRY`).
type ContributorWindow struct {
	From  *time.Time
	Until *time.Time
	// Revoked reports Until as an expiry edge's date rather than the claim's own — an early
	// expiry someone asked for.
	Revoked bool
}

// Admits reports at as falling inside the window, which `R-C4KEY` requires of every claim
// the key signs.
func (w ContributorWindow) Admits(at time.Time) bool {
	if w.From != nil && at.Before(*w.From) {
		return false
	}
	return w.Until == nil || !at.After(*w.Until)
}

// String renders the window for a listing, each open end as a dash.
func (w ContributorWindow) String() string {
	span := windowBound(w.From) + " … " + windowBound(w.Until)
	if w.Revoked {
		return span + " (expiry requested)"
	}
	return span
}

// bound renders one end, an absent one as open.
func windowBound(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.UTC().Format(time.RFC3339)
}

// Windows reads the window of each contributor claim in held, keyed by id. The carriers of an
// expiry edge are the `contribution/expiry` claims and the contributor claims themselves,
// a successor key's claim being free to retire its predecessor (`R-DEXPIRY`).
func ContributorWindows(held, expiries []ranke.Claim) (map[string]ContributorWindow, error) {
	carriers := append(append([]ranke.Claim{}, expiries...), held...)
	windows := make(map[string]ContributorWindow, len(held))
	for _, c := range held {
		w, err := ContributorWindowOf(c, carriers)
		if err != nil {
			return nil, err
		}
		windows[c.ID().String()] = w
	}
	return windows, nil
}

// ContributorWindowOf reads the window of the contributor claim self, with carriers searched for the
// expiry edges naming it. An expiry only shortens the end, and the earliest of several wins,
// so a revocation cannot extend a key someone already retired.
func ContributorWindowOf(self ranke.Claim, carriers []ranke.Claim) (ContributorWindow, error) {
	from, err := keyBound(self, ranke.FieldPubkeyValidFrom)
	if err != nil {
		return ContributorWindow{}, err
	}
	until, err := keyBound(self, ranke.FieldPubkeyExpiresAfter)
	if err != nil {
		return ContributorWindow{}, err
	}
	w := ContributorWindow{From: from, Until: until}
	revoked, err := revocation(self.ID(), carriers)
	if err != nil {
		return ContributorWindow{}, err
	}
	if revoked != nil && (w.Until == nil || revoked.Before(*w.Until)) {
		w.Until, w.Revoked = revoked, true
	}
	return w, nil
}

// revocation is the earliest end any expiry edge imposes on the contributor at id, or nil
// where none names it. The date sits on the edge, not on the claim carrying it.
func revocation(id ranke.Id, carriers []ranke.Claim) (*time.Time, error) {
	var earliest *time.Time
	for _, c := range carriers {
		for _, e := range c.Edges() {
			if e.Type() != ranke.EdgeTypeExpiry || !e.Reference().Equal(id) {
				continue
			}
			at, err := edgeBound(e, ranke.FieldPubkeyExpiresAfter)
			if err != nil {
				return nil, fmt.Errorf("ranke/client: read the expiry on %s: %w", c.ID(), err)
			}
			if at != nil && (earliest == nil || at.Before(*earliest)) {
				earliest = at
			}
		}
	}
	return earliest, nil
}

// keyBound reads one bound off a claim, absent reported as nil.
func keyBound(self ranke.Claim, field string) (*time.Time, error) {
	if !self.Node().HasField(field) {
		return nil, nil
	}
	raw, err := self.Node().GetField(field)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: read %s of %s: %w", field, self.ID(), err)
	}
	return parseBound(field, raw)
}

// edgeBound reads one bound off an edge, absent reported as nil.
func edgeBound(e ranke.Edge, field string) (*time.Time, error) {
	raw, err := e.GetField(field)
	if err != nil {
		return nil, nil
	}
	return parseBound(field, raw)
}

// parseBound reads a bound in the one timestamp form every field takes (`V-TIME`).
func parseBound(field, raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	at, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return nil, fmt.Errorf("ranke/client: %s is not a timestamp: %q", field, raw)
	}
	at = at.UTC()
	return &at, nil
}
