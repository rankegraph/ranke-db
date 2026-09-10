package rest_http

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	ranke "github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/openapi"
)

// TestListBranchesNeedsNoBranchName is the route's reason to exist: a client told nothing
// about an archive learns what it may address.
func TestListBranchesNeedsNoBranchName(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	seedBranches(t, h, "main", "notes")

	got := listBranches(t, h, http.StatusOK)
	if len(got.Branches) < 2 {
		t.Fatalf("branches = %+v, want at least main and notes", got.Branches)
	}
	byName := map[string]string{}
	for _, b := range got.Branches {
		if b.Head == "" {
			t.Errorf("branch %q listed with no head", b.Name)
		}
		byName[b.Name] = b.Head
	}
	for _, want := range []string{"main", "notes"} {
		if byName[want] == "" {
			t.Errorf("branch %q missing from %+v", want, got.Branches)
		}
	}
}

// TestFreshArchiveListsItsFoundingBranch pins that a fresh instance is explorable, and
// that founding leaves something to explore: the branch it bound the first contributor
// to. An archive with none would be one nobody could write to.
func TestFreshArchiveListsItsFoundingBranch(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	got := listBranches(t, h, http.StatusOK)
	if len(got.Branches) != 1 || got.Branches[0].Name != "main" {
		t.Fatalf("branches = %+v, want just the founding branch", got.Branches)
	}
	if got.Branches[0].Head == "" {
		t.Error("the founding branch has no head")
	}
}

// TestListBranchesNeedsRightOnBranches pins that dropping the `$` from no path dropped the
// grant with it: the listing is gated on the reserved target core-access names, and the
// refusal is the access decision rather than a not-found.
func TestListBranchesNeedsRightOnBranches(t *testing.T) {
	h, _ := newServingStack(t, []string{"CR *", "C $branches", "R $universe", "R $archive"})
	seedBranches(t, h, "main")
	listBranches(t, h, http.StatusForbidden)
}

// TestScopeRoutesAreSeparatelyGated pins one grant per scope. Each route names its scope
// as a plain segment, and each is refused without the grant that scope requires.
func TestScopeRoutesAreSeparatelyGated(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		grants     []string
	}{
		{"universe without R $universe", "/universe/claims/", []string{"CR *", "C $branches", "R $archive", "R $branches"}},
		{"archive without R $archive", "/archive/claims/", []string{"CR *", "C $branches", "R $universe", "R $branches"}},
		// An ordinary glob reaches neither: matchBranch requires an exact grant on a
		// `$`-prefixed target, and this locks that in rather than relying on it.
		{"an ordinary glob confers no reserved scope", "/universe/claims/", []string{"R *"}},
		{"an ordinary glob confers no archive scope", "/archive/claims/", []string{"R *"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newServingStack(t, tc.grants)
			rec := do(t, h, http.MethodGet, tc.path+testClaimID)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// TestUniverseReachesBeyondTheHeadsClosure is the test that would have caught the two
// scopes calling the same thing. A claim the Universe holds but the current head does not
// reach is the whole point of the privileged read: it is how an archive is reached from a
// Universe and a head id alone.
func TestUniverseReachesBeyondTheHeadsClosure(t *testing.T) {
	h, store := newServingStack(t, everyReadRight)
	self, priv, _ := seedBranches(t, h, "main")

	// Signed like any other claim, but never contributed — so it is in the Universe and
	// outside every branch table.
	loose := signedClaim(t, self, priv, "source/note", "outside every closure")
	if err := store.PutClaims(context.Background(), []ranke.Claim{loose}); err != nil {
		t.Fatalf("PutClaims: %v", err)
	}

	if rec := do(t, h, http.MethodGet, "/universe/claims/"+loose.ID().String()); rec.Code != http.StatusOK {
		t.Fatalf("universe scope = %d, want 200 — the unconfined read cannot reach it: %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, h, http.MethodGet, "/archive/claims/"+loose.ID().String()); rec.Code != http.StatusNotFound {
		t.Fatalf("archive scope = %d, want 404 — it is outside the head's closure", rec.Code)
	}
}

// TestArchiveScopeSpansBranches pins what `/archive/…` is for: a claim is reachable
// without the client knowing which branch holds it.
func TestArchiveScopeSpansBranches(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	self, priv, selfClaim := seedBranches(t, h, "main")

	onNotes := signedClaim(t, self, priv, "source/note", "a claim on notes")
	contribute(t, h, "notes", selfClaim, onNotes)

	if rec := do(t, h, http.MethodGet, "/archive/claims/"+onNotes.ID().String()); rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 without naming a branch: %s", rec.Code, rec.Body.String())
	}
	// And the branch that does not hold it says so.
	if rec := do(t, h, http.MethodGet, "/branches/main/claims/"+onNotes.ID().String()); rec.Code != http.StatusNotFound {
		t.Fatalf("main = %d, want 404 — the claim is on notes", rec.Code)
	}
}

// TestReservedNameInBranchPositionIsNotASecondRoute pins one route per scope. `{branch}`
// means an ordinary branch, so a reserved name there names one that does not exist.
func TestReservedNameInBranchPositionIsNotASecondRoute(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	seedBranches(t, h, "main")

	for _, name := range []string{"$archive", "$universe", "$branches"} {
		rec := do(t, h, http.MethodGet, "/branches/"+name+"/claims/"+testClaimID)
		if rec.Code != http.StatusNotFound {
			t.Errorf("/branches/%s/claims/… = %d, want 404", name, rec.Code)
		}
	}
}

// TestBranchNamesShadowNoFixedRoute is what moving branches off the root bought. A branch
// may be called anything, including the name of a collection or a fixed route.
func TestBranchNamesShadowNoFixedRoute(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	seedBranches(t, h, "branches", "health", "claims", "query", "system")

	// The collection still lists, and its own name is a member like any other.
	got := listBranches(t, h, http.StatusOK)
	if len(got.Branches) < 5 {
		t.Fatalf("branches = %+v, want all five", got.Branches)
	}

	for _, name := range []string{"branches", "health", "claims", "query", "system"} {
		rec := do(t, h, http.MethodGet, "/branches/"+name+"/head")
		if rec.Code != http.StatusOK {
			t.Errorf("/branches/%s/head = %d, want 200: %s", name, rec.Code, rec.Body.String())
		}
	}

	// And the fixed routes they are named after resolve unaffected.
	if rec := do(t, h, http.MethodGet, "/health"); rec.Code != http.StatusOK {
		t.Errorf("/health = %d, want 200 with a branch of that name present", rec.Code)
	}
}

// TestPinnedRoutesSurviveAShell pins the reason `$` left the path. These routes exist to be
// typed into curl, and a path that becomes a different path when typed is not that.
func TestPinnedRoutesSurviveAShell(t *testing.T) {
	// Anything a POSIX shell would expand, glob or quote-strip in an unquoted word.
	expandable := regexp.MustCompile(`[$*?\[\]{}~` + "`" + `'"\\|&;<>()! ]`)

	spec, err := os.ReadFile("../../../openapi/openapi.yaml")
	if err != nil {
		t.Fatalf("read the contract: %v", err)
	}
	pathKey := regexp.MustCompile(`(?m)^  (/\S*):$`)
	found := pathKey.FindAllStringSubmatch(string(spec), -1)
	if len(found) == 0 {
		t.Fatal("no paths found in the contract — the pattern no longer matches it")
	}
	for _, m := range found {
		// A templated segment is the client's to fill; the literal path is what gets typed.
		literal := regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(m[1], "x")
		if expandable.MatchString(literal) {
			t.Errorf("path %q carries a character a shell would expand", m[1])
		}
	}
}

// TestCachePosture pins the headers the contract declares, and that the conditional
// request it promises actually answers 304 rather than resending the body.
func TestCachePosture(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	self, priv, selfClaim := seedBranches(t, h, "main")
	claim := signedClaim(t, self, priv, "source/note", "cached")
	contribute(t, h, "main", selfClaim, claim)
	id := claim.ID().String()

	t.Run("a by-id read is immutable", func(t *testing.T) {
		rec := do(t, h, http.MethodGet, "/branches/main/claims/"+id)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("ETag"); got != `"`+id+`"` {
			t.Errorf("ETag = %q, want the claim id — it content-addresses the bytes", got)
		}
		if got := rec.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
			t.Errorf("Cache-Control = %q, want immutable", got)
		}
	})

	t.Run("a matching validator is not resent", func(t *testing.T) {
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/branches/main/claims/"+id, nil)
		r.Header.Set("If-None-Match", `"`+id+`"`)
		h.ServeHTTP(rec, r)
		if rec.Code != http.StatusNotModified {
			t.Fatalf("status = %d, want 304", rec.Code)
		}
		if rec.Body.Len() != 0 {
			t.Errorf("304 carried %d bytes of body", rec.Body.Len())
		}
	})

	t.Run("a moving target revalidates", func(t *testing.T) {
		for _, path := range []string{"/branches", "/branches/main/head"} {
			rec := do(t, h, http.MethodGet, path)
			if rec.Code != http.StatusOK {
				t.Fatalf("%s = %d: %s", path, rec.Code, rec.Body.String())
			}
			etag := rec.Header().Get("ETag")
			if !strings.HasPrefix(etag, "W/") {
				t.Errorf("%s ETag = %q, want a weak validator", path, etag)
			}
			if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
				t.Errorf("%s Cache-Control = %q, want no-cache", path, got)
			}

			again := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.Header.Set("If-None-Match", etag)
			h.ServeHTTP(again, r)
			if again.Code != http.StatusNotModified {
				t.Errorf("%s conditional = %d, want 304", path, again.Code)
			}
		}
	})
}

// TestInfoRoutes pins what /info answers beyond a head id, and that the archive head — the
// one a client needs to name the $archive scope — is reported nowhere else.
func TestInfoRoutes(t *testing.T) {
	h, _ := newServingStack(t, everyReadRight)
	seedBranches(t, h, "main")

	var branch openapi.BranchInfo
	decode(t, do(t, h, http.MethodGet, "/branches/main/info"), &branch)
	if branch.Name != "main" {
		t.Errorf("name = %q, want main", branch.Name)
	}
	if branch.Head == "" {
		t.Error("no head reported")
	}
	if branch.Height == 0 {
		t.Error("height = 0 — the head of a seeded branch references its contribution, so it climbs")
	}
	if branch.UpdatedAt.IsZero() {
		t.Error("no updatedAt — a branch that moved has a head with a created_at")
	}

	var arch openapi.ArchiveInfo
	decode(t, do(t, h, http.MethodGet, "/archive/info"), &arch)
	if arch.Head == "" {
		t.Fatal("no archive head — nothing else in the contract reports it")
	}
	if arch.Head == branch.Head {
		t.Error("the archive head equals the branch head; the branch table is a claim of its own")
	}
	if arch.Branches != 1 {
		t.Errorf("branches = %d, want 1", arch.Branches)
	}

	// The head a query would be rooted at: reading the archive scope by that id resolves.
	if rec := do(t, h, http.MethodGet, "/archive/claims/"+arch.Head); rec.Code != http.StatusOK {
		t.Errorf("the reported archive head does not resolve in its own scope: %d", rec.Code)
	}
}

// TestInfoNeedsItsScopeRight pins that each info route is gated by the scope it reads.
func TestInfoNeedsItsScopeRight(t *testing.T) {
	h, _ := newServingStack(t, []string{"CR *", "C $branches", "R $branches"})
	seedBranches(t, h, "main")
	if rec := do(t, h, http.MethodGet, "/archive/info"); rec.Code != http.StatusForbidden {
		t.Fatalf("archive info without R $archive = %d, want 403", rec.Code)
	}
}

// --- helpers --------------------------------------------------------------

// decode reads a JSON body, failing on a non-200 or an unparseable one.
func decode(t *testing.T, rec *httptest.ResponseRecorder, into any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
}

// do issues one request against the handler.
func do(t *testing.T, h http.Handler, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

// listBranches reads GET /branches, asserting the status and decoding the body on 200.
func listBranches(t *testing.T, h http.Handler, want int) openapi.BranchList {
	t.Helper()
	rec := do(t, h, http.MethodGet, "/branches")
	if rec.Code != want {
		t.Fatalf("GET /branches = %d, want %d: %s", rec.Code, want, rec.Body.String())
	}
	var got openapi.BranchList
	if want == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode branch list: %v (body %s)", err, rec.Body.String())
		}
	}
	return got
}

// seedBranches gives each named branch a claim, so it exists in the branch table, and
// returns the contributing identity for further claims.
func seedBranches(t *testing.T, h http.Handler, names ...string) (ranke.Contributor, ed25519.PrivateKey, ranke.Claim) {
	t.Helper()
	self, priv, selfClaim := testContributor(t)
	for _, name := range names {
		contribute(t, h, name, selfClaim, signedClaim(t, self, priv, "source/note", "seed "+name))
	}
	return self, priv, selfClaim
}

// contribute merges claims onto a branch through the endpoint, the way a client does.
func contribute(t *testing.T, h http.Handler, branch string, claims ...ranke.Claim) {
	t.Helper()
	var body bytes.Buffer
	w := ranke.NewWireWriter(&body, ranke.WireConstraints{
		Branches:     []string{branch},
		Referencable: []string{branch},
		Creatable:    []string{branch},
	})
	for _, claim := range claims {
		if err := w.WriteClaim(branch, claim); err != nil {
			t.Fatalf("WriteClaim: %v", err)
		}
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/contribute", bytes.NewReader(body.Bytes()))
	r.Header.Set("Content-Type", "application/cbor-seq")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /contribute = %d, want 201: %s", rec.Code, rec.Body.String())
	}
}

// testContributor mints a client-side identity: its own key, its own signed contributor
// claim. The client signs the claims; the server attests only the merge.
func testContributor(t *testing.T) (ranke.Contributor, ed25519.PrivateKey, ranke.Claim) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	encoded, err := ranke.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encode public key: %v", err)
	}
	claim, err := ranke.NewClaim(ranke.NodeContributor, nil).
		WithInlineContent(encoded).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(time.Unix(0, 0).UTC()).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign contributor claim: %v", err)
	}
	self, err := claim.AsContributor(context.Background(), nil, priv)
	if err != nil {
		t.Fatalf("AsContributor: %v", err)
	}
	return self, priv, claim
}

// signedClaim signs one inline-text claim. Height 1: it references its contributor, an initial
// node at 0, and a referencing claim must declare its height.
func signedClaim(t *testing.T, self ranke.Contributor, priv ed25519.PrivateKey, typ, text string) ranke.Claim {
	t.Helper()
	claim, err := ranke.NewClaim(typ, self).
		WithInlineContent([]byte(text)).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(1).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign %s: %v", typ, err)
	}
	return claim
}

// TestWhoamiNeedsNoGrant is the route's reason to exist: an account holding nothing
// still learns what it authenticated as, so "why is this 403" is one request rather
// than a hunt through someone else's configuration.
func TestWhoamiNeedsNoGrant(t *testing.T) {
	h, _ := newServingStack(t, []string{})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/system/whoami", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /system/whoami = %d, want 200 for a grantless account: %s", rec.Code, rec.Body)
	}

	var got struct {
		Account string   `json:"account"`
		Grants  []string `json:"grants"`
		Caveats []string `json:"caveats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Account == "" {
		t.Error("account is empty; the caller authenticated as somebody")
	}
	// Empty arrays rather than null: a client ranging over them needs no nil check.
	if got.Grants == nil || got.Caveats == nil {
		t.Errorf("grants=%v caveats=%v, want empty arrays", got.Grants, got.Caveats)
	}
}

// TestWhoamiReportsTheGrantsItHolds pins that the specs come back as configured, so a
// client can read its own reach rather than probing branch by branch.
func TestWhoamiReportsTheGrantsItHolds(t *testing.T) {
	h, _ := newServingStack(t, []string{"CR foo_*", "R $archive"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/system/whoami", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /system/whoami = %d, want 200: %s", rec.Code, rec.Body)
	}

	var got struct {
		Grants []string `json:"grants"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	held := map[string]bool{}
	for _, g := range got.Grants {
		held[g] = true
	}
	for _, want := range []string{"CR foo_*", "R $archive"} {
		if !held[want] {
			t.Errorf("grant %q missing from %v", want, got.Grants)
		}
	}
}
