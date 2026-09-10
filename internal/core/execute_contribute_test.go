package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	ranke "github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/internal/core/access"
)

// newContributor mints a client-side contributor: its own key, its own signed
// contributor claim. This is the application's identity, not the server's signing
// identity — the client signs the claims, the server attests only the merge.
func newContributor(t *testing.T, u ranke.Universe) (ranke.Contributor, ed25519.PrivateKey, ranke.Claim) {
	t.Helper()
	ctx := context.Background()

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
	self, err := claim.AsContributor(ctx, u, priv)
	if err != nil {
		t.Fatalf("AsContributor: %v", err)
	}
	// The contributor claim travels in the contribution too: every claim it signs
	// references it, so an archive that has never seen it cannot resolve the closure.
	return self, priv, claim
}

// contributorHeight is where a contributor claim sits: it references nothing, so the
// verifier treats it as an initial node at 0, and a claim citing it sits one above.
// (The normative spec §R-HEIGHT puts an initial node at 1; the library is the oracle for
// what verifies, so these follow it. Reported as a divergence.)
const contributorHeight = 0

// TestContributeRejectsAPartlyReadableBody pins that a body only partly understood is
// refused whole: a contribution is atomic, so guessing at one record would merge
// something the client did not send.
func TestContributeRejectsAPartlyReadableBody(t *testing.T) {
	c := newStack(t)
	for _, tc := range []struct{ name, body string }{
		{"empty", ""},
		{"not cbor at all", "this is not a contribution"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &Request{Op: OpClaimContribute, Body: strings.NewReader(tc.body)}
			_, err := c.Handle(context.Background(), req)
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("err = %v, want ErrInvalidRequest", err)
			}
			if got := Categorize(err); got != CatInvalid {
				t.Fatalf("category = %q, want %q", got, CatInvalid)
			}
		})
	}
}

// TestContributeMerges drives the whole arm: a client-signed claim crosses the wire
// format, the sequencer merges it, and the new head plus the contributed ids come back.
func TestContributeMerges(t *testing.T) {
	c := newStack(t)
	self, priv, selfClaim := newContributor(t, c.store)

	claim, err := ranke.NewClaim("source/letter", self).
		WithInlineContent([]byte("a claim the client signed")).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(contributorHeight + 1).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign claim: %v", err)
	}

	body := writeContribution(t, "main", selfClaim, claim)
	req := &Request{Op: OpClaimContribute, Body: bytes.NewReader(body)}
	out, mediaType := serve(t, c, req)
	if mediaType != mediaJSON {
		t.Fatalf("content type = %q, want %q", mediaType, mediaJSON)
	}

	var got Contribution
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v (body %s)", err, out)
	}
	if got.Head == "" {
		t.Fatal("no new head returned")
	}
	if len(got.Ids) == 0 {
		t.Fatal("no contributed ids returned")
	}

	// And the claim is now readable on the branch it was merged onto.
	read, _ := serve(t, c, &Request{Op: OpClaimGet, Branch: "main", ClaimID: claim.ID()})
	want, err := claim.Envelope()
	if err != nil {
		t.Fatalf("Envelope: %v", err)
	}
	if !bytes.Equal(read, want) {
		t.Fatal("the merged claim does not read back as the bytes the client signed")
	}
}

// TestContributeSpansSeveralBranches pins what the body naming its own branches buys: one
// contribution advances more than one branch, and each needs the C right.
func TestContributeSpansSeveralBranches(t *testing.T) {
	c := newStack(t)
	self, priv, selfClaim := newContributor(t, c.store)

	var body bytes.Buffer
	w := ranke.NewWireWriter(&body, ranke.WireConstraints{
		Branches:     []string{"main", "notes"},
		Referencable: []string{"main", "notes"},
		Creatable:    []string{"main", "notes"},
	})
	// The contributor claim goes to both: each branch is verified as its own graph, so a
	// branch whose claims reference it must hold it, or that closure is incomplete.
	for _, branch := range []string{"main", "notes"} {
		if err := w.WriteClaim(branch, selfClaim); err != nil {
			t.Fatalf("WriteClaim: %v", err)
		}
	}
	for _, branch := range []string{"main", "notes"} {
		claim, err := ranke.NewClaim("source/letter", self).
			WithInlineContent([]byte("a claim for " + branch)).
			WithEncoding(ranke.EncodingOctetStream).
			WithHeight(contributorHeight + 1).
			Sign(priv)
		if err != nil {
			t.Fatalf("sign claim: %v", err)
		}
		if err := w.WriteClaim(branch, claim); err != nil {
			t.Fatalf("WriteClaim: %v", err)
		}
	}

	out, _ := serve(t, c, &Request{Op: OpClaimContribute, Body: bytes.NewReader(body.Bytes())})
	var got Contribution
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v (body %s)", err, out)
	}
	if got.Head == "" {
		t.Fatal("no new head returned")
	}

	// Both branches now resolve.
	for _, branch := range []string{"main", "notes"} {
		if _, err := c.Handle(context.Background(), &Request{Op: OpBranchHead, Branch: branch}); err != nil {
			t.Fatalf("branch %q did not advance: %v", branch, err)
		}
	}
}

// writeContribution writes claims onto one branch, as a client would.
func writeContribution(t *testing.T, branch string, claims ...ranke.Claim) []byte {
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
	return body.Bytes()
}

// TestContributeAuthorizesTheDeclarationBeforeReading pins what the header buys: the C
// right is settled from the declaration alone. The reader here records any read past the
// header, so the test only passes if nothing beyond it was consumed.
func TestContributeAuthorizesTheDeclarationBeforeReading(t *testing.T) {
	c := newStack(t)
	chk, err := access.New(map[string][]string{"ops": {"CR foo_*"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	c.access = chk

	var header bytes.Buffer
	declared := ranke.WireConstraints{
		Branches:     []string{"secret"},
		Referencable: []string{"secret"},
		Creatable:    []string{"secret"},
	}
	if err := ranke.NewWireWriter(&header, declared).WriteContent(ranke.ContentBlob{
		Hash: mustHash(t, []byte("x")), Content: []byte("x"),
	}); err != nil {
		t.Fatalf("WriteContent: %v", err)
	}
	body := &failAfter{header: header.Bytes()}

	_, err = c.Handle(context.Background(), &Request{Op: OpClaimContribute, Body: body})
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden from the declaration alone", err)
	}
	if body.readPast {
		t.Fatal("the body was read past its header before the grant was checked")
	}
}

// failAfter yields the header bytes, then records any further read.
type failAfter struct {
	header   []byte
	off      int
	readPast bool
}

func (f *failAfter) Read(p []byte) (int, error) {
	if f.off >= len(f.header) {
		f.readPast = true
		return 0, io.EOF
	}
	n := copy(p, f.header[f.off:])
	f.off += n
	return n, nil
}

// mustHash addresses content the way a claim does.
func mustHash(t *testing.T, b []byte) ranke.Id {
	t.Helper()
	h, err := ranke.HashContent(b)
	if err != nil {
		t.Fatalf("HashContent: %v", err)
	}
	return h
}

// TestContributeRefusesReservedTypes pins that a client cannot mint what the Sequencer
// reserves. The account here holds CRUD on ordinary branches and on $branches, so the
// branch table is lifted for it and refused for the two limiting claims — which is the
// grant translating into a capability, and only that grant.
func TestContributeRefusesReservedTypes(t *testing.T) {
	for _, nodeType := range []string{ranke.NodeDelete, ranke.NodeExpiry} {
		t.Run(nodeType, func(t *testing.T) {
			c := newStack(t)
			self, priv, selfClaim := newContributor(t, c.store)

			reserved, err := ranke.NewClaim(nodeType, self).
				WithInlineContent([]byte("a claim only the Sequencer may mint")).
				WithEncoding(ranke.EncodingOctetStream).
				WithHeight(contributorHeight + 1).
				Sign(priv)
			if err != nil {
				t.Fatalf("sign claim: %v", err)
			}

			body := writeContribution(t, "main", selfClaim, reserved)
			_, err = c.Handle(context.Background(), &Request{
				Op: OpClaimContribute, Body: bytes.NewReader(body),
			})
			if err == nil {
				t.Fatalf("a %s claim was accepted from a client", nodeType)
			}
		})
	}
}

// TestContributeBoundsWhatItMayReference pins step 3 at the seam v0.11.0 opened: a
// contribution may reference only claims its grants reach. The account here writes to
// "open" but reads nothing else, so citing a claim that lives on another branch is refused
// — the Sequencer enforcing a scope the server composed, without knowing an account exists.
func TestContributeBoundsWhatItMayReference(t *testing.T) {
	c := newStack(t)
	self, priv, selfClaim := newContributor(t, c.store)

	// First, land a claim on "private" as an account that may write there.
	seed, err := ranke.NewClaim("source/letter", self).
		WithInlineContent([]byte("a claim on a branch the next account cannot read")).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(contributorHeight + 1).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign seed: %v", err)
	}
	if _, err := c.Handle(context.Background(), &Request{
		Op:   OpClaimContribute,
		Body: bytes.NewReader(writeContribution(t, "private", selfClaim, seed)),
	}); err != nil {
		t.Fatalf("seeding contribution: %v", err)
	}

	// Now an account that may write and read "open" and nothing else, so the only thing
	// standing between it and the merge is what it may reference.
	chk, err := access.New(map[string][]string{"ops": {"CR open"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	c.access = chk

	citing, err := ranke.NewClaim("derivation/summary", self).
		WithInlineContent([]byte("cites a claim it may not read")).
		WithEncoding(ranke.EncodingOctetStream).
		WithEdges(mustEdge(t, "derivation/source", seed.ID())).
		WithHeight(contributorHeight + 2).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign citing claim: %v", err)
	}

	_, err = c.Handle(context.Background(), &Request{
		Op:   OpClaimContribute,
		Body: bytes.NewReader(writeContribution(t, "open", selfClaim, citing)),
	})
	if err == nil {
		t.Fatal("a contribution referenced a claim outside every branch its grants reach")
	}
}

// mustEdge builds one typed reference.
func mustEdge(t *testing.T, edgeType string, ref ranke.Id) ranke.Edge {
	t.Helper()
	e, err := ranke.NewEdge(ranke.EdgeConfig{Type: edgeType, Reference: ref})
	if err != nil {
		t.Fatalf("NewEdge: %v", err)
	}
	return e
}

// TestBranchTableGrantLiftsItsType pins the one translation between the two vocabularies:
// C on $branches is the DB's name for administering the branch table, and the library's
// name for the same capability is a lifted contribution/branches type. Without the grant
// the type is refused; with it the claim is admitted, so the grant is exercisable.
func TestBranchTableGrantLiftsItsType(t *testing.T) {
	refusedWithout, admittedWith := false, false

	for _, tc := range []struct {
		name   string
		grants []string
		lifted bool
	}{
		{"without the grant", []string{"CRUD *"}, false},
		{"with the grant", []string{"CRUD *", "C $branches"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newStack(t)
			chk, err := access.New(map[string][]string{"ops": tc.grants})
			if err != nil {
				t.Fatalf("access.New: %v", err)
			}
			c.access = chk

			got := c.liftedTypes(&Request{Principal: access.Principal{Account: "ops"}})
			has := len(got) == 1 && got[0] == ranke.NodeBranches
			if has != tc.lifted {
				t.Fatalf("lifted = %v, want the branch-table type present = %v", got, tc.lifted)
			}
			if tc.lifted {
				admittedWith = true
			} else {
				refusedWithout = true
			}
		})
	}

	if !refusedWithout || !admittedWith {
		t.Fatal("both directions must be exercised for the translation to mean anything")
	}
}

// TestContributeGatesBranchCreation pins that bringing a branch into being is its own
// right: the server matches the declared branches against the base, and only an account
// holding C over the branch table may create the ones missing. Writing to a branch that
// already exists needs nothing extra.
func TestContributeGatesBranchCreation(t *testing.T) {
	c := newStack(t)
	self, priv, selfClaim := newContributor(t, c.store)

	claim, err := ranke.NewClaim("source/letter", self).
		WithInlineContent([]byte("the first claim on a branch")).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(contributorHeight + 1).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign claim: %v", err)
	}
	body := func() *bytes.Reader {
		return bytes.NewReader(writeContribution(t, "fresh", selfClaim, claim))
	}

	// May write the branch, may not add it to the table.
	withoutTable, err := access.New(map[string][]string{"ops": {"CR fresh"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	c.access = withoutTable
	if _, err := c.Handle(context.Background(), &Request{Op: OpClaimContribute, Body: body()}); err == nil {
		t.Fatal("a branch was created without the right to create one")
	}

	// With it, the same contribution lands.
	withTable, err := access.New(map[string][]string{"ops": {"CR fresh", "C $branches"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	c.access = withTable
	if _, err := c.Handle(context.Background(), &Request{Op: OpClaimContribute, Body: body()}); err != nil {
		t.Fatalf("contribution refused with the branch-table right: %v", err)
	}

	// And a second contribution to the now-existing branch needs no such right.
	next, err := ranke.NewClaim("source/letter", self).
		WithInlineContent([]byte("a second claim, the branch now existing")).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(contributorHeight + 1).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign second claim: %v", err)
	}
	c.access = withoutTable
	if _, err := c.Handle(context.Background(), &Request{
		Op:   OpClaimContribute,
		Body: bytes.NewReader(writeContribution(t, "fresh", selfClaim, next)),
	}); err != nil {
		t.Fatalf("writing an existing branch needed the creation right: %v", err)
	}
}

// TestContributeNamesAVerificationFailureAsTheCallers: a claim breaking a rule is the
// contribution's defect, so the refusal is invalid and carries what verification said —
// which claim, and which rule. It answered internal before, dressing a malformed claim as
// a fault of the server's own.
func TestContributeNamesAVerificationFailureAsTheCallers(t *testing.T) {
	c := newStack(t)
	self, priv, selfClaim := newContributor(t, c.store)

	// A height no reference supports: the contributor sits at 0, so 1 is the only value
	// `V-HEIGHT` admits, and the verifier re-derives it.
	claim, err := ranke.NewClaim("source/letter", self).
		WithInlineContent([]byte("a claim carrying the wrong height")).
		WithEncoding(ranke.EncodingOctetStream).
		WithHeight(contributorHeight + 5).
		Sign(priv)
	if err != nil {
		t.Fatalf("sign claim: %v", err)
	}

	body := writeContribution(t, "main", selfClaim, claim)
	_, err = c.Handle(context.Background(), &Request{
		Op: OpClaimContribute, Body: bytes.NewReader(body),
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want ErrInvalidRequest", err)
	}
	if got := Categorize(err); got != CatInvalid {
		t.Fatalf("category = %q, want %q", got, CatInvalid)
	}
	if got := err.Error(); !strings.Contains(got, "height") {
		t.Errorf("the refusal reads %q, and a caller correcting the claim needs the rule "+
			"verification named", got)
	}
}
