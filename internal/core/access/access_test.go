package access

import "testing"

// TestAllow walks the core-access scenarios: a grant permits a matching right on a
// matching branch, an out-of-glob or un-granted request is denied, the reserved
// names are privileged, an ordinary glob never reaches them, an unknown account is
// denied, and the cross-branch delete rule falls out of per-branch calls.
func TestAllow(t *testing.T) {
	c, err := New(map[string][]string{
		"webapp":      {"CR foo_*"},
		"provisioner": {"C $branches"},
		"backup":      {"R $universe"},
		"archivist":   {"R $archive"},
		"seqop":       {"D $sequencer"},
		"wildcard":    {"R *"},
		"deleter":     {"D a"}, // holds D on branch a, not b
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []struct {
		name  string
		acct  string
		right Right
		br    string
		want  bool
	}{
		{"contribute within glob", "webapp", Contribute, "foo_bar", true},
		{"read within glob", "webapp", Read, "foo_bar", true},
		{"right not granted", "webapp", Update, "foo_bar", false},
		{"branch outside glob", "webapp", Read, "bar-baz", false},
		{"branch-table admin", "provisioner", Contribute, Branches, true},
		{"privileged universe read", "backup", Read, Universe, true},
		{"archive-wide read", "archivist", Read, Archive, true},
		{"privileged sequencer op", "seqop", Delete, Sequencer, true},
		{"ordinary glob misses universe", "wildcard", Read, Universe, false},
		{"ordinary glob misses archive", "wildcard", Read, Archive, false},
		{"ordinary glob misses sequencer", "wildcard", Read, Sequencer, false},
		{"ordinary glob matches branch", "wildcard", Read, "anything", true},
		{"unknown account", "ghost", Read, "foo_bar", false},
		{"delete on held branch", "deleter", Delete, "a", true},
		{"delete on other branch", "deleter", Delete, "b", false},
	}
	for _, tc := range cases {
		p := Principal{Account: tc.acct}
		if got := c.Allow(p, tc.right, tc.br); got != tc.want {
			t.Errorf("%s: Allow(%s, %c, %s) = %v, want %v", tc.name, tc.acct, tc.right, tc.br, got, tc.want)
		}
	}
}

// TestAllowCaveats covers attenuation: a caveat narrows an account's grants to
// their intersection, never widens them.
func TestAllowCaveats(t *testing.T) {
	c, err := New(map[string][]string{"webapp": {"CR foo_*"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	attenuated := Principal{Account: "webapp", Caveats: []Grant{mustGrant(t, "R foo_bar")}}

	cases := []struct {
		name  string
		right Right
		br    string
		want  bool
	}{
		{"caveat permits the narrowed read", Read, "foo_bar", true},
		{"caveat withholds contribute", Contribute, "foo_bar", false},
		{"caveat withholds a sibling branch", Read, "foo-qux", false},
	}
	for _, tc := range cases {
		if got := c.Allow(attenuated, tc.right, tc.br); got != tc.want {
			t.Errorf("%s: = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A caveat cannot grant what the account never held.
	widen := Principal{Account: "webapp", Caveats: []Grant{mustGrant(t, "D foo_bar")}}
	if c.Allow(widen, Delete, "foo_bar") {
		t.Fatal("caveat widened the account beyond its grants")
	}
}

// TestAllowCaveatsIntersect covers successive attenuation: a bearer holding "R
// foo_*" narrows further to "R foo_bar" before handing the token on, and the two
// caveats must intersect (AND), not union (OR) — otherwise the second narrowing
// step would hand the recipient back the first caveat's wider authority.
func TestAllowCaveatsIntersect(t *testing.T) {
	c, err := New(map[string][]string{"webapp": {"CR foo_*"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	narrowedTwice := Principal{
		Account: "webapp",
		Caveats: []Grant{mustGrant(t, "R foo_*"), mustGrant(t, "R foo_bar")},
	}

	if !c.Allow(narrowedTwice, Read, "foo_bar") {
		t.Fatal("both caveats permit foo_bar, want allowed")
	}
	if c.Allow(narrowedTwice, Read, "foo-other") {
		t.Fatal("second caveat withholds foo-other, want denied — caveats must intersect, not union")
	}
}

func mustGrant(t *testing.T, spec string) Grant {
	t.Helper()
	g, err := ParseGrant(spec)
	if err != nil {
		t.Fatalf("ParseGrant(%q): %v", spec, err)
	}
	return g
}

// TestParseGrantRejects covers the grant specs ParseGrant must reject at build
// (config-syntax) time — the dropped A right, an unknown reserved name, and branch
// names outside the lowercase/digit/'-' alphabet.
func TestParseGrantRejects(t *testing.T) {
	bad := map[string]string{
		"unknown right letter": "CX foo_*",
		"admin is not a right": "A foo_*",
		"missing glob":         "CR",
		"non-R on universe":    "CR $universe",
		"unknown reserved":     "R $secret",
		"uppercase branch":     "R Foo",
		"hyphen branch":        "R foo-bar",
		"leading underscore":   "R _foo",
		"slash in branch":      "R foo/bar",
	}
	for name, spec := range bad {
		if _, err := ParseGrant(spec); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	if _, err := New(map[string][]string{"": {"R foo_*"}}); err == nil {
		t.Error("empty account name: want error")
	}
}

// TestParseGrantReserved confirms the reserved names parse, with $universe held to
// R-only and the others accepting CRUD (pending their final right-sets).
func TestParseGrantReserved(t *testing.T) {
	for _, spec := range []string{"R $universe", "R $archive", "CRUD $archive", "C $branches", "CRUD $branches", "D $sequencer"} {
		if _, err := ParseGrant(spec); err != nil {
			t.Errorf("ParseGrant(%q): unexpected error %v", spec, err)
		}
	}
}

// TestPaperAccessExample pins the paper's own example (§Access Control): with
// `webapp CR foo_*, provisioner C $branches`, provisioner creates branches such as foo_bar
// and webapp reads and contributes to them. Neither grant confers the other's job, and
// $branches carries no glob — it is one server-wide surface.
func TestPaperAccessExample(t *testing.T) {
	c, err := New(map[string][]string{
		"webapp":      {"CR foo_*"},
		"provisioner": {"C $branches"},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, tc := range []struct {
		name  string
		acct  string
		right Right
		br    string
		want  bool
	}{
		{"provisioner creates branches", "provisioner", Contribute, Branches, true},
		{"provisioner does not write foo_bar", "provisioner", Contribute, "foo_bar", false},
		{"provisioner does not read foo_bar", "provisioner", Read, "foo_bar", false},
		{"webapp contributes to foo_bar", "webapp", Contribute, "foo_bar", true},
		{"webapp reads foo_bar", "webapp", Read, "foo_bar", true},
		{"webapp cannot create a branch", "webapp", Contribute, Branches, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Principal{Account: tc.acct}
			if got := c.Allow(p, tc.right, tc.br); got != tc.want {
				t.Fatalf("Allow(%s, %c, %s) = %v, want %v", tc.acct, tc.right, tc.br, got, tc.want)
			}
		})
	}
}

// TestGrantStringRoundTrips: whoami reports grants and caveats through String, so what
// it emits has to be what ParseGrant accepts — otherwise a client reads a spec it could
// not send back, and an attenuated token reports caveats in a form nothing understands.
func TestGrantStringRoundTrips(t *testing.T) {
	for _, spec := range []string{
		"R foo_bar", "CR foo_*", "CRUD *", "R $universe", "C $branches", "R $archive",
	} {
		g, err := ParseGrant(spec)
		if err != nil {
			t.Fatalf("ParseGrant(%q): %v", spec, err)
		}
		again, err := ParseGrant(g.String())
		if err != nil {
			t.Fatalf("ParseGrant(%q.String() = %q): %v", spec, g.String(), err)
		}
		if again.String() != g.String() {
			t.Errorf("%q: round trip gave %q then %q", spec, g.String(), again.String())
		}
	}
}

// TestGrantStringOrdersRightsCRUD: one set of rights has one spelling, so a client
// comparing what whoami reported against what it configured sees no spurious difference.
func TestGrantStringOrdersRightsCRUD(t *testing.T) {
	g, err := ParseGrant("RC foo_bar")
	if err != nil {
		t.Fatalf("ParseGrant: %v", err)
	}
	if got := g.String(); got != "CR foo_bar" {
		t.Errorf("String() = %q, want %q", got, "CR foo_bar")
	}
}
