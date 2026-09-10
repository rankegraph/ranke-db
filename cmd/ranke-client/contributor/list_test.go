package contributor

import (
	"strings"
	"testing"
	"time"

	"github.com/rankegraph/ranke-db/client"
)

// TestStatusReadsTheWindowAtTheDate: the listing answers the question `R-C4KEY` asks — would
// a claim signed now verify under this contributor — rather than printing bounds alone.
func TestStatusReadsTheWindowAtTheDate(t *testing.T) {
	at := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	future := at.AddDate(1, 0, 0)
	past := at.AddDate(-1, 0, 0)
	for _, tc := range []struct {
		name string
		w    client.ContributorWindow
		want string
	}{
		{"open at both ends", client.ContributorWindow{}, "valid now"},
		{"opens later", client.ContributorWindow{From: &future}, "not yet valid"},
		{"closed already", client.ContributorWindow{Until: &past}, "lapsed"},
		{"inside", client.ContributorWindow{From: &past, Until: &future}, "valid now"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := status(tc.w, at); !strings.Contains(got, tc.want) {
				t.Errorf("status = %q, want it to say %q", got, tc.want)
			}
		})
	}
}
