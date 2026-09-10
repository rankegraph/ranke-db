// package: client / transport
// type:    test
// job:     the key window `R-DEXPIRY` fixes — the bounds a contributor states, the expiry edge
// that shortens them, and the date `R-C4KEY` asks about
// limits:  the reading; the server's own verification is ranke-go's
package client_test

import (
	"testing"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/client"
)

// day is a date in the fixture's year, so a case states a window without a timestamp.
func day(month time.Month, n int) time.Time {
	return time.Date(2026, month, n, 0, 0, 0, 0, time.UTC)
}

// bounded builds a contributor claim over key with the bounds given, either omitted where
// zero — a contributor carrying neither is valid at any date.
func bounded(t *testing.T, key ranke.Keypair, from, until time.Time) ranke.Claim {
	t.Helper()
	b := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(key.Pubkey).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC())
	if !from.IsZero() {
		b = b.WithField(ranke.FieldPubkeyValidFrom, ranke.FormatTimestamp(from))
	}
	if !until.IsZero() {
		b = b.WithField(ranke.FieldPubkeyExpiresAfter, ranke.FormatTimestamp(until))
	}
	claim, err := b.Sign(key.Private)
	if err != nil {
		t.Fatalf("sign a contributor: %v", err)
	}
	return claim
}

// TestWindowAdmitsWithinItsBounds walks the four shapes a window takes, each bound standing
// on its own. A claim dated outside its key's window fails verification (`R-C4KEY`), so this
// is the test that decides which contributor a resolve may take.
func TestWindowAdmitsWithinItsBounds(t *testing.T) {
	key := newKeypair(t)
	for _, tc := range []struct {
		name        string
		from, until time.Time
		at          time.Time
		admits      bool
	}{
		{"no bounds at all", time.Time{}, time.Time{}, day(time.June, 1), true},
		{"inside both", day(time.January, 1), day(time.December, 31), day(time.June, 1), true},
		{"before from", day(time.July, 1), time.Time{}, day(time.June, 1), false},
		{"after until", time.Time{}, day(time.May, 1), day(time.June, 1), false},
		{"on the closing bound", time.Time{}, day(time.June, 1), day(time.June, 1), true},
		{"on the opening bound", day(time.June, 1), time.Time{}, day(time.June, 1), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w, err := client.ContributorWindowOf(bounded(t, key, tc.from, tc.until), nil)
			if err != nil {
				t.Fatalf("ContributorWindowOf: %v", err)
			}
			if got := w.Admits(tc.at); got != tc.admits {
				t.Errorf("Admits(%s) = %v, want %v — window %s", stampOf(tc.at), got, tc.admits, w)
			}
		})
	}
}

// TestAnExpiryShortensTheWindow: an expiry edge naming the contributor moves the end of the
// window to its own date, and the earliest of several wins, so a revocation can never extend
// a key someone already retired (`R-DEXPIRY`).
func TestAnExpiryShortensTheWindow(t *testing.T) {
	key := newKeypair(t)
	self := bounded(t, key, time.Time{}, day(time.December, 31))
	revoker := newKeypair(t)

	for _, tc := range []struct {
		name  string
		dates []time.Time
		until time.Time
	}{
		{"one, earlier", []time.Time{day(time.March, 1)}, day(time.March, 1)},
		{"one, later than the claim's own", []time.Time{day(time.December, 31).AddDate(1, 0, 0)}, day(time.December, 31)},
		{"several, the earliest wins", []time.Time{day(time.August, 1), day(time.February, 1)}, day(time.February, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var carriers []ranke.Claim
			for _, at := range tc.dates {
				carriers = append(carriers, expiryAgainst(t, revoker, self.ID(), at))
			}
			w, err := client.ContributorWindowOf(self, carriers)
			if err != nil {
				t.Fatalf("ContributorWindowOf: %v", err)
			}
			if w.Until == nil || !w.Until.Equal(tc.until) {
				t.Fatalf("window ends at %v, want %s", w.Until, stampOf(tc.until))
			}
			if shortened := tc.until.Before(day(time.December, 31)); w.Revoked != shortened {
				t.Errorf("Revoked = %v, want %v — an expiry set the end only when it shortened it",
					w.Revoked, shortened)
			}
		})
	}
}

// TestAnExpiryElsewhereLeavesTheWindow: an expiry names its target, so one against another
// contributor is not this key's business.
func TestAnExpiryElsewhereLeavesTheWindow(t *testing.T) {
	key, other := newKeypair(t), newKeypair(t)
	self := bounded(t, key, time.Time{}, time.Time{})
	elsewhere := bounded(t, other, time.Time{}, time.Time{})
	w, err := client.ContributorWindowOf(self, []ranke.Claim{expiryAgainst(t, other, elsewhere.ID(), day(time.March, 1))})
	if err != nil {
		t.Fatalf("ContributorWindowOf: %v", err)
	}
	if w.Until != nil || w.Revoked {
		t.Errorf("window ends at %v (revoked %v), want open — the expiry names another contributor",
			w.Until, w.Revoked)
	}
}

// stampOf renders a date for a failure message.
func stampOf(at time.Time) string { return at.UTC().Format(time.RFC3339) }
