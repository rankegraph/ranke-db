package core

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	ranke "github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// newStack assembles a real stack through the same ports config uses — an in-memory
// universe, a dev sequencer over it, a real signer — because the execute stage is a seam
// onto the library and a double would leave exactly that seam untested.
func newStack(t *testing.T) *Core {
	t.Helper()
	ctx := context.Background()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sig, err := signer.New(ctx, scope.Literal(map[string]string{"type": "inmemory", "key": string(key)}))
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}

	store := ranke.NewMemoryUniverse()
	seq, err := sequencer.New(ctx,
		scope.Literal(map[string]string{"type": "dev", "seed": t.Name()}), store, sig, nil)
	if err != nil {
		t.Fatalf("sequencer.New: %v", err)
	}
	foundArchive(t, seq)

	return newCoreFor(t, seq, store, WithSigner(sig),
		WithLayers([]StorageLayer{{Name: "hot", Type: "mem"}, {Name: "cold", Type: "fs"}}))
}

// newCoreFor wires a core admitting one account that holds every read right, so these
// tests exercise execution rather than access.
func newCoreFor(t *testing.T, seq sequencer.Sequencer, store ranke.Universe, opts ...Option) *Core {
	t.Helper()
	ctx := context.Background()

	a, err := auth.New(ctx, scope.Literal(map[string]string{"type": "noauth", "subject": "ops"}))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{"ops": {"CRUD *", "R $universe", "R $archive", "CRUD $branches"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	return New(set, chk, seq, store, opts...)
}

// serve runs a request and returns the body its stream produced.
func serve(t *testing.T, c *Core, req *Request) ([]byte, string) {
	t.Helper()
	stream, err := c.Handle(context.Background(), req)
	if err != nil {
		t.Fatalf("Handle(%v): %v", req.Op, err)
	}
	defer func() { _ = stream.Close() }()
	var body bytes.Buffer
	if _, err := stream.WriteTo(&body); err != nil {
		t.Fatalf("WriteTo(%v): %v", req.Op, err)
	}
	return body.Bytes(), stream.ContentType()
}

// TestHealthAnswersOnABrokenStack pins that health does not depend on an archive: it is
// wanted precisely when the stack is too broken to open one. The core here has no
// sequencer and no storage at all, only a signer.
func TestHealthAnswersOnABrokenStack(t *testing.T) {
	ctx := context.Background()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	key := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	sig, err := signer.New(ctx, scope.Literal(map[string]string{"type": "inmemory", "key": string(key)}))
	if err != nil {
		t.Fatalf("signer.New: %v", err)
	}

	c := newCoreFor(t, nil, nil, WithSigner(sig))
	body, mediaType := serve(t, c, &Request{Op: OpHealthGet})

	if mediaType != mediaJSON {
		t.Fatalf("content type = %q, want %q", mediaType, mediaJSON)
	}
	var got Health
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal health: %v", err)
	}
	if got.Status != "ok" {
		t.Fatalf("status = %q, want ok", got.Status)
	}
	if !strings.HasPrefix(got.Signer, "ed25519:") {
		t.Fatalf("signer = %q, want the signing identity", got.Signer)
	}
}

// TestReadArms drives each read against the real stack. A freshly bootstrapped archive
// holds the sequencer's own claims, so these assert the shape of what comes back rather
// than particular content.
func TestReadArms(t *testing.T) {
	c := newStack(t)

	t.Run("branch list", func(t *testing.T) {
		body, mediaType := serve(t, c, &Request{Op: OpBranchList, Branch: access.Branches})
		if mediaType != mediaJSON {
			t.Fatalf("content type = %q, want %q", mediaType, mediaJSON)
		}
		var got branchList
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatalf("unmarshal: %v (body %s)", err, body)
		}
	})

	t.Run("query serves a framed sequence", func(t *testing.T) {
		q := ranke.Query{Select: ranke.Select{Branch: ranke.BranchArchive}}
		body, mediaType := serve(t, c, &Request{Op: OpClaimQuery, Branch: ranke.BranchArchive, Query: &q})
		if mediaType != mediaJSONSeq {
			t.Fatalf("content type = %q, want %q", mediaType, mediaJSONSeq)
		}
		// RFC 7464: every record opens with RS. An empty result set writes nothing.
		if len(body) > 0 && body[0] != recordSeparator {
			t.Fatalf("json-seq body does not open with RS: %q", body[:1])
		}
	})

	t.Run("an unknown claim is not found", func(t *testing.T) {
		id, err := ranke.HashContent([]byte("unknown-claim-fixture"))
		if err != nil {
			t.Fatalf("HashContent: %v", err)
		}
		_, err = c.Handle(context.Background(), &Request{Op: OpClaimGet, Branch: Universe, ClaimID: id})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
		if got := Categorize(err); got != CatNotFound {
			t.Fatalf("category = %q, want %q", got, CatNotFound)
		}
	})

	t.Run("an unknown branch is not found", func(t *testing.T) {
		_, err := c.Handle(context.Background(), &Request{Op: OpBranchHead, Branch: "no_such_branch"})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})
}

// TestQueryEncodingIsExplicit pins that the engine is always asked for a serialized
// form: its native kinds carry Go objects, which the server must never encode itself.
func TestQueryEncodingIsExplicit(t *testing.T) {
	c := newStack(t)
	for _, tc := range []struct {
		name string
		enc  ranke.ResultEncoding
		want string
	}{
		{"unset defaults to json", "", mediaJSONSeq},
		{"native is treated as unset", ranke.ResultNative, mediaJSONSeq},
		{"cbor frames as a cbor sequence", ranke.ResultCBOR, mediaCBORSeq},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := ranke.Query{
				Select: ranke.Select{Branch: ranke.BranchArchive},
				Output: ranke.Output{Encoding: tc.enc},
			}
			_, mediaType := serve(t, c, &Request{Op: OpClaimQuery, Branch: ranke.BranchArchive, Query: &q})
			if mediaType != tc.want {
				t.Fatalf("content type = %q, want %q", mediaType, tc.want)
			}
		})
	}
}

// TestCanonicalFormReachesTheWireUnaltered is the guarantee the whole serve path exists
// for: the bytes a client receives under detail/form/encoding = claims/original/cbor are
// the bytes the library produced, which is what the claim's id signs. Anything the server
// re-encoded would break here.
func TestCanonicalFormReachesTheWireUnaltered(t *testing.T) {
	c := newStack(t)
	ctx := context.Background()

	archive, err := c.archive(ctx)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	head, err := archive.GetClaim(ctx, archive.Head())
	if err != nil {
		t.Fatalf("GetClaim(head): %v", err)
	}
	want, err := head.Envelope()
	if err != nil {
		t.Fatalf("Envelope: %v", err)
	}

	body, mediaType := serve(t, c, &Request{Op: OpClaimGet, Branch: Universe, ClaimID: archive.Head()})
	if mediaType != mediaCBOR {
		t.Fatalf("content type = %q, want %q", mediaType, mediaCBOR)
	}
	if !bytes.Equal(body, want) {
		t.Fatalf("claim bytes differ from the library's canonical form (%d vs %d bytes)", len(body), len(want))
	}

	// And the bytes still verify against the id they were fetched by.
	decoded, err := ranke.DecodeClaim(archive.Head(), body)
	if err != nil {
		t.Fatalf("the served bytes do not verify against the id: %v", err)
	}
	if !decoded.ID().Equal(archive.Head()) {
		t.Fatalf("decoded id = %s, want %s", decoded.ID(), archive.Head())
	}
}

// TestQueryDetailEnvelopeReturnsStoredBytes pins detail: envelope end to end, through
// /query rather than the by-id GET TestCanonicalFormReachesTheWireUnaltered already
// covers: the result carries the exact stored envelope bytes (R-QCANON), not a
// re-encoded claim.
func TestQueryDetailEnvelopeReturnsStoredBytes(t *testing.T) {
	c := newStack(t)
	ctx := context.Background()

	archive, err := c.archive(ctx)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	head, err := archive.GetClaim(ctx, archive.Head())
	if err != nil {
		t.Fatalf("GetClaim(head): %v", err)
	}
	want, err := head.Envelope()
	if err != nil {
		t.Fatalf("Envelope: %v", err)
	}

	q := ranke.Query{
		Select: ranke.Select{Branch: ranke.BranchArchive},
		Output: ranke.Output{Detail: ranke.DetailEnvelope, Encoding: ranke.ResultCBOR},
	}
	body, mediaType := serve(t, c, &Request{Op: OpClaimQuery, Branch: ranke.BranchArchive, Query: &q})
	if mediaType != mediaCBORSeq {
		t.Fatalf("content type = %q, want %q", mediaType, mediaCBORSeq)
	}
	if !bytes.Contains(body, want) {
		t.Fatal("query body does not carry the head's stored envelope bytes verbatim")
	}
}

// TestLayersReportNameAndTypeOnly pins that introspection leaks no address, credential
// or path — the layer list is what config retained, which is a name and a type.
func TestLayersReportNameAndTypeOnly(t *testing.T) {
	c := newStack(t)
	body, _ := serve(t, c, &Request{Op: OpLayerList})

	var got layerList
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Layers) != 2 {
		t.Fatalf("layers = %+v, want 2", got.Layers)
	}
	if got.Layers[0].Name != "hot" || got.Layers[0].Type != "mem" {
		t.Fatalf("first layer = %+v, want hot/mem", got.Layers[0])
	}
	// Every field a layer carries is asserted above, so a field added later that
	// could carry a secret fails this rather than slipping onto the wire.
	var raw []map[string]any
	if err := json.Unmarshal(mustField(t, body, "layers"), &raw); err != nil {
		t.Fatalf("unmarshal raw layers: %v", err)
	}
	for _, layer := range raw {
		if len(layer) != 2 {
			t.Fatalf("layer carries %d fields (%v), want name and type only", len(layer), layer)
		}
	}
}

// TestWritesNeedASequencer pins that a core with no sequencer cannot serve a write: the
// archive every operation opens is the sequencer's, so there is nothing to write onto.
func TestWritesNeedASequencer(t *testing.T) {
	c := newCoreFor(t, nil, ranke.NewMemoryUniverse())

	for _, op := range []Operation{OpClaimContribute, OpClaimDelete} {
		_, err := c.Handle(context.Background(), &Request{Op: op, Branch: "foo"})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("%v err = %v, want ErrNotImplemented", op, err)
		}
		if got := Categorize(err); got != CatUnimplemented {
			t.Fatalf("%v category = %q, want %q", op, got, CatUnimplemented)
		}
	}
}

// TestDevClockAdvanceNeedsWiring pins that OpDevClockAdvance refuses on a core no one
// called WithDevClock on — no --dev, or a sequencer.type other than "dev" (config's
// own guard); the route must not silently no-op against a production stack.
func TestDevClockAdvanceNeedsWiring(t *testing.T) {
	c := newCoreFor(t, nil, ranke.NewMemoryUniverse())
	_, err := c.Handle(context.Background(), &Request{Op: OpDevClockAdvance})
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("err = %v, want ErrNotImplemented", err)
	}
	if got := Categorize(err); got != CatUnimplemented {
		t.Fatalf("category = %q, want %q", got, CatUnimplemented)
	}
}

// TestDevClockAdvanceReachesTheWiredClock pins the wiring itself: WithDevClock's func
// is what the operation calls, and its return is what the response reports back.
func TestDevClockAdvanceReachesTheWiredClock(t *testing.T) {
	var got time.Time
	advance := func(t time.Time) time.Time {
		got = t
		return t.Add(time.Hour) // a distinguishable answer, so the test can't pass by accident
	}
	c := newCoreFor(t, nil, ranke.NewMemoryUniverse(), WithDevClock(advance))

	want := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	body, _ := serve(t, c, &Request{Op: OpDevClockAdvance, DevClockAt: want})

	if !got.Equal(want) {
		t.Fatalf("wired func received %s, want %s", got, want)
	}
	var reported devClock
	if err := json.Unmarshal(body, &reported); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if wantReported := want.Add(time.Hour); !reported.Time.Equal(wantReported) {
		t.Fatalf("response time = %s, want %s", reported.Time, wantReported)
	}
}

// mustField pulls one raw JSON field out of an object.
func mustField(t *testing.T, body []byte, name string) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		t.Fatalf("unmarshal object: %v", err)
	}
	raw, ok := fields[name]
	if !ok {
		t.Fatalf("field %q missing from %s", name, body)
	}
	return raw
}

// discard keeps io imported for the stream contract's WriterTo use above.
var _ io.Writer = io.Discard

// foundArchive brings the archive into being, which construction no longer does: a
// Sequencer over an empty bookmark list reports InGenesis and refuses every other call
// until Found succeeds.
func foundArchive(t *testing.T, seq sequencer.Sequencer) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate founding key: %v", err)
	}
	encoded, err := ranke.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encode founding key: %v", err)
	}
	if _, err := seq.Found(context.Background(), encoded, "main"); err != nil {
		t.Fatalf("found the archive: %v", err)
	}
}
