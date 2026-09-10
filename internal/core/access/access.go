// package: access / policy
// type:    checker
// job:     decide whether a system account may exercise a CRUD right on a branch
// limits:  pure policy from config; no ports, no ctx; core loops it for delete (-> config, core)
//
// Access is this deployment's policy: accounts holding CRUD grants over branch globs,
// declared in config, never in the graph. The checker answers one (principal, right,
// branch) question; verifiability never consults it.
//
// A for admin is C on $branches, the branch table being a claim, so creating a branch
// contributes to it while writing into one is the separate C on that branch. A caveat is
// a grant of opposite polarity, the effective permission their intersection.
package access

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Right is one CRUD access right.
type Right byte

const (
	Contribute Right = 'C' // contribute claims to the branch (creating it if new)
	Read       Right = 'R' // read the branch
	Update     Right = 'U' // overlay an existing claim with a newer version
	Delete     Right = 'D' // delete claims (needs D on every branch that holds the claim)
)

// The reserved branches a grant may target. '$' is illegal in an ordinary name, so no
// glob confers one by accident.
const (
	Universe  = "$universe"
	Archive   = "$archive"
	Sequencer = "$sequencer"
	Branches  = "$branches"
)

var reserved = map[string]bool{Universe: true, Archive: true, Sequencer: true, Branches: true}

// branchGlob is the charset ValidateBranchName holds a name to (`R-FIELDS`) plus
// '*'/'?', aligned so no grant can name a branch that cannot exist.
var branchGlob = regexp.MustCompile(`^[a-z0-9_*?]+$`)

// rightset is a bitmask over the CRUD rights.
type rightset uint8

// bit maps a right to its bit, reporting whether it is a valid CRUD letter.
func bit(r Right) (rightset, bool) {
	switch r {
	case Contribute:
		return 1 << 0, true
	case Read:
		return 1 << 1, true
	case Update:
		return 1 << 2, true
	case Delete:
		return 1 << 3, true
	}
	return 0, false
}

// Grant confers rights over the branches matching a glob — the unit of both an account
// grant and a token caveat, the Checker applying the polarity.
type Grant struct {
	rights rightset
	glob   string
}

// ParseGrant parses one "RIGHTS glob" spec ("CR foo_*", "R $universe"), rejecting
// unknown letters, malformed globs, and non-R rights on $universe. Caveats reuse it.
func ParseGrant(spec string) (Grant, error) {
	fields := strings.Fields(spec)
	if len(fields) != 2 {
		return Grant{}, fmt.Errorf("grant %q: want \"RIGHTS glob\"", spec)
	}
	letters, glob := fields[0], fields[1]

	var rs rightset
	for _, ch := range letters {
		b, ok := bit(Right(ch))
		if !ok {
			return Grant{}, fmt.Errorf("grant %q: unknown right %q", spec, string(ch))
		}
		rs |= b
	}

	if strings.HasPrefix(glob, "$") {
		if !reserved[glob] {
			return Grant{}, fmt.Errorf("grant %q: unknown reserved branch %q", spec, glob)
		}
		// $universe is read-only (paper); the other reserved names accept any CRUD
		// pending the access model's sign-off.
		if glob == Universe {
			if readOnly, _ := bit(Read); rs != readOnly {
				return Grant{}, fmt.Errorf("grant %q: only R applies to %s", spec, Universe)
			}
		}
	} else if strings.HasPrefix(glob, "_") {
		// ValidateBranchName reserves a leading underscore.
		return Grant{}, fmt.Errorf("grant %q: branch %q may not begin with '_'", spec, glob)
	} else if !branchGlob.MatchString(glob) {
		return Grant{}, fmt.Errorf("grant %q: branch %q must be lowercase letters, digits and '_' (with * or ? wildcards)", spec, glob)
	}

	return Grant{rights: rs, glob: glob}, nil
}

// String renders the grant back into the "RIGHTS glob" spec it parsed from, letters in
// CRUD order so one set of rights has one spelling. ParseGrant accepts what this emits.
func (g Grant) String() string {
	var letters strings.Builder
	for _, r := range []Right{Contribute, Read, Update, Delete} {
		if b, _ := bit(r); g.rights&b != 0 {
			letters.WriteByte(byte(r))
		}
	}
	return letters.String() + " " + g.glob
}

// Allows reports whether this grant carries right and its glob matches branch.
func (g Grant) Allows(right Right, branch string) bool {
	b, ok := bit(right)
	if !ok || g.rights&b == 0 {
		return false
	}
	return matchBranch(g.glob, branch)
}

// Principal is the identity a request acts as: the account the credential resolved to,
// plus any caveats attenuating its grants (empty = none).
type Principal struct {
	Account string
	Caveats []Grant
}

// Checker answers access requests against a fixed set of accounts and grants.
type Checker struct {
	accounts map[string][]Grant
}

// New builds a checker from the configured accounts, each mapping to compact grant
// specs. It validates every grant offline and fails on the first malformed one.
func New(accounts map[string][]string) (*Checker, error) {
	c := &Checker{accounts: make(map[string][]Grant, len(accounts))}
	for name, specs := range accounts {
		if name == "" {
			return nil, fmt.Errorf("access: empty account name")
		}
		for _, spec := range specs {
			g, err := ParseGrant(spec)
			if err != nil {
				return nil, fmt.Errorf("access: account %q: %w", name, err)
			}
			c.accounts[name] = append(c.accounts[name], g)
		}
	}
	return c, nil
}

// Grants reports the specs held by one account, for a principal asking what it may do.
// An unknown account holds none, which is what the checker denies on.
func (c *Checker) Grants(account string) []string {
	held := c.accounts[account]
	specs := make([]string, 0, len(held))
	for _, g := range held {
		specs = append(specs, g.String())
	}
	return specs
}

// Allow reports whether the principal may exercise right on branch: the account's grants
// and every caveat must allow it. Unknown or ungranted is denied.
//
// Caveats are successive attenuations, each a predicate the request must still satisfy,
// so one Grant carries a whole step's rights ("RIGHTS glob") — siblings would read as
// alternatives, and a second narrowing would fail to narrow.
func (c *Checker) Allow(p Principal, right Right, branch string) bool {
	if !anyAllows(c.accounts[p.Account], right, branch) {
		return false
	}
	return allAllows(p.Caveats, right, branch)
}

// anyAllows reports whether any grant in the set allows the action.
func anyAllows(grants []Grant, right Right, branch string) bool {
	for _, g := range grants {
		if g.Allows(right, branch) {
			return true
		}
	}
	return false
}

// allAllows reports whether every grant in the set allows the action — vacuously
// true with no caveats, since an absent caveat withholds nothing.
func allAllows(grants []Grant, right Right, branch string) bool {
	for _, g := range grants {
		if !g.Allows(right, branch) {
			return false
		}
	}
	return true
}

// matchBranch matches a grant glob against a branch. A "$..." name needs an exact
// literal grant, so "*" never reaches it; ordinary branches match by shell glob.
func matchBranch(glob, branch string) bool {
	if strings.HasPrefix(branch, "$") {
		return glob == branch
	}
	ok, err := path.Match(glob, branch)
	return err == nil && ok
}
