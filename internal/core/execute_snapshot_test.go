package core

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	ranke "github.com/rankegraph/ranke-go"
)

// countingSequencer records how often a request opened a snapshot.
type countingSequencer struct {
	ranke.Sequencer
	mu    sync.Mutex
	opens int
}

func (s *countingSequencer) GetArchive(ctx context.Context) (ranke.Archive, error) {
	s.mu.Lock()
	s.opens++
	s.mu.Unlock()
	return s.Sequencer.GetArchive(ctx)
}

// TestOneSnapshotPerRequest pins what makes the branch listing internally consistent: the
// heads in one response come from one archive, so they cannot straddle two points in time.
// Re-opening per branch would answer with a mix the moment a merge landed mid-listing —
// which no assertion on the values could catch, and this counts instead.
func TestOneSnapshotPerRequest(t *testing.T) {
	c := newStack(t)
	counting := &countingSequencer{Sequencer: c.seq}
	c.seq = counting

	self, priv, selfClaim := newContributor(t, c.store)
	for _, branch := range []string{"main", "notes", "drafts"} {
		claim, err := ranke.NewClaim("source/letter", self).
			WithInlineContent([]byte("a claim for " + branch)).
			WithEncoding(ranke.EncodingOctetStream).
			WithHeight(contributorHeight + 1).
			Sign(priv)
		if err != nil {
			t.Fatalf("sign claim: %v", err)
		}
		body := writeContribution(t, branch, selfClaim, claim)
		serve(t, c, &Request{Op: OpClaimContribute, Body: bytes.NewReader(body)})
	}

	counting.mu.Lock()
	counting.opens = 0
	counting.mu.Unlock()

	out, _ := serve(t, c, &Request{Op: OpBranchList, Branch: Branches})

	var got struct {
		Branches []struct{ Name, Head string } `json:"branches"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v (body %s)", err, out)
	}
	if len(got.Branches) != 3 {
		t.Fatalf("branches = %+v, want three", got.Branches)
	}

	counting.mu.Lock()
	opens := counting.opens
	counting.mu.Unlock()
	if opens != 1 {
		t.Fatalf("the listing opened %d snapshots, want exactly 1", opens)
	}
}
