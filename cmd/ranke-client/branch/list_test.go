package branch

import (
	"strings"
	"testing"
)

// TestRenderAlignsTheNames: the heads are the column an operator reads down, so the names
// are padded to the longest rather than each line finding its own width.
func TestRenderAlignsTheNames(t *testing.T) {
	var out strings.Builder
	render(&out, "bciqarchive", []row{
		{Name: "main", Head: "bciqone"},
		{Name: "ranke_website", Head: "bciqtwo"},
	})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want the archive and two branches: %q", len(lines), out.String())
	}
	if lines[0] != "archive: bciqarchive" {
		t.Errorf("first line is %q, want the archive head — the only route that reports it", lines[0])
	}
	first, second := strings.Index(lines[1], "bciqone"), strings.Index(lines[2], "bciqtwo")
	if first != second {
		t.Errorf("heads start at column %d and %d, want one column: %q", first, second, out.String())
	}
}

// TestRenderSaysWhenThereAreNoBranches: a founded archive with nothing on it yet answers
// with an empty table, which is an answer rather than a failure.
func TestRenderSaysWhenThereAreNoBranches(t *testing.T) {
	var out strings.Builder
	render(&out, "bciqarchive", nil)
	if !strings.Contains(out.String(), "no branches") {
		t.Errorf("an empty table rendered as %q, want it said so", out.String())
	}
}

// TestRenderCarriesTheDetail: --deep adds a second line per branch, under the name rather
// than beside the head, so the head column stays readable.
func TestRenderCarriesTheDetail(t *testing.T) {
	var out strings.Builder
	render(&out, "", []row{{Name: "main", Head: "bciqone", Detail: "height 4, moved 2026-09-10T11:17:54Z"}})
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want the branch and its detail: %q", len(lines), out.String())
	}
	if strings.Contains(lines[1], "main") {
		t.Errorf("the detail line repeats the name: %q", lines[1])
	}
	if !strings.Contains(lines[1], "height 4") {
		t.Errorf("the detail line is %q, want the height and when it moved", lines[1])
	}
	// No archive head means no archive line: an account may read $branches without it.
	if strings.Contains(out.String(), "archive:") {
		t.Errorf("an archive line was printed for an unread head: %q", out.String())
	}
}
