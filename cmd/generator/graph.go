// package: main / cmd
// type:    logic
// job:     the fixture identity and the graph shapes it signs
// limits:  builds claims only; delivering them is the client's (-> client.go)
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/rankegraph/ranke-go"
)

// batch is one contribution: the branch it lands on and the claims it carries.
type batch struct {
	branch string
	claims []ranke.Claim
}

// batches are the contributions in the order they merge, so the branch table grows a
// revision per batch — and, where they name different branches, more than one branch.
type batches []batch

// branchNames are the branches a generated archive spreads over. An archive with one branch
// exercises nothing about branches, so a fixture has several; the first takes most of the
// contributions, as a main line does.
var branchNames = []string{"main", "notes", "review", "entities", "ingest"}

// branchesFor returns the first n names, with the primary first.
func branchesFor(primary string, n int) []string {
	names := []string{primary}
	for _, name := range branchNames {
		if len(names) >= n {
			break
		}
		if name != primary {
			names = append(names, name)
		}
	}
	return names
}

// identityDomain separates derived fixture keys from anything else derived from a name.
const identityDomain = "ranke-db/generator/contributor/"

// epoch is the fixture clock's start. Pinned, not time.Now(): an id covers created_at,
// so a fixed clock makes the same command yield the same ids.
var epoch = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// clockStep advances the fixture clock per claim, so created_at orders the build.
const clockStep = time.Minute

// grower signs claims for one identity, remembering them so later claims can cite them.
// A shape needing several signing identities derives them with attest.
type grower struct {
	self      ranke.Contributor
	selfClaim ranke.Claim
	rnd       *rand.Rand
	at        time.Time
	made      []made
}

// signer is an identity claims are attributed to: the contributor claim naming its key, and
// the Contributor that signs with it. The root grower is one; attest derives the others.
type signer struct {
	name  string
	claim ranke.Claim
	as    ranke.Contributor
}

// attest derives an identity from a name and has the root vouch for it: the claim declares
// the new key and the root signs it, which is what makes the root an authority over it
// (§5.7). The identity then signs its own claims, so a reader can tell whose they are.
//
// The key claim cites the actor it belongs to, which is how the operational side reaches the
// semantic one: a contributor is a key, an entity is a person or a machine, and they never
// share a node — so a claim asserting the connection is the only thing that links them.
func (g *grower) attest(ctx context.Context, name string, of made) (*signer, error) {
	priv, err := identityKey(name)
	if err != nil {
		return nil, err
	}
	pub, err := ranke.EncodePublicKey(priv.Public())
	if err != nil {
		return nil, fmt.Errorf("encode public key for %q: %w", name, err)
	}
	edge, err := ranke.NewEdge(ranke.EdgeConfig{
		Reference:  of.claim.ID(),
		Referenced: of.claim,
		Type:       edgeIdentity,
	})
	if err != nil {
		return nil, fmt.Errorf("edge to %s: %w", of.claim.ID(), err)
	}
	// Signed by the root, whose key the builder takes from the contributor it is attributed
	// to. The height is resolved against both claims it reaches: the actor it cites and the
	// root that signs it.
	claim, err := ranke.NewClaim(ranke.NodeTypeContributor, g.self).
		WithInlineContent(pub).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(g.at).
		WithHeightResolver(ctx, ranke.HeightsFrom(of.claim, g.selfClaim)).
		WithEdges(edge).
		Sign()
	if err != nil {
		return nil, fmt.Errorf("sign contributor claim for %q: %w", name, err)
	}
	g.at = g.at.Add(clockStep)
	// The pubkey is inline, so resolving it needs no Universe; the key it is checked
	// against is the one this identity will sign with.
	as, err := claim.AsContributor(ctx, nil, priv)
	if err != nil {
		return nil, fmt.Errorf("bind contributor %q: %w", name, err)
	}
	return &signer{name: name, claim: claim, as: as}, nil
}

// rootSigner is the grower's own identity as a signer, so one helper writes claims for
// either the root or an attested actor.
func (g *grower) rootSigner() *signer {
	return &signer{name: "root", claim: g.selfClaim, as: g.self}
}

// cite is one edge a claim carries: what it reaches, and how. The type is the edge's own —
// a claim names its inputs, mentions an entity it did not produce, and says who decided,
// and those are three different relations rather than three inputs.
type cite struct {
	to  made
	typ string
	dir ranke.RelationDirection
}

// asInputs turns claims into the derivation/input edges that carry provenance (§3.5).
func asInputs(refs ...made) []cite {
	out := make([]cite, 0, len(refs))
	for _, ref := range refs {
		out = append(out, cite{to: ref, typ: edgeInput})
	}
	return out
}

// The edge types the release scenario uses beyond the input edge every derivation carries.
// A relation edge states its direction (§4.7); these all run from the claim outward.
const (
	edgeInput = "derivation/input"
	// A key claim cites the actor it belongs to: provenance, since the key exists because
	// that actor does.
	edgeIdentity = "derivation/identity"
	// What a scan found, and who decided. Neither is an input: a scan does not derive from
	// the vulnerability it reports, and a decision does not derive from the person who made
	// it. Reification is for n:n relations between entities (foundation paper §Relations);
	// a claim naming one entity says so with an edge, and `to` marks the entity as the
	// object of what the claim states.
	edgeMentions  = "relation/mentions"
	edgeDecidedBy = "relation/decided_by"
)

// spec is one claim to sign: who it is attributed to, where it lands, what it says, and
// what it reaches. Fields carry what a reader filters on, which content cannot.
type spec struct {
	by       *signer
	branch   string
	typ      string
	content  []byte
	encoding string
	fields   map[string]string
	cites    []cite
	at       time.Time // explicit schedule (release.go); zero rides the ambient clock
}

// write signs one claim from a spec and remembers it. The height is resolved against every
// claim it reaches — the contributor it is signed by included, since a claim signed by an
// attested identity sits above that identity's own claim.
func (g *grower) write(s spec) (made, error) {
	edges := make([]ranke.Edge, 0, len(s.cites))
	refs := make([]ranke.Claim, 0, len(s.cites)+1)
	refs = append(refs, s.by.claim)
	for _, c := range s.cites {
		edge, err := ranke.NewEdge(ranke.EdgeConfig{
			Reference:         c.to.claim.ID(),
			Referenced:        c.to.claim,
			Type:              c.typ,
			RelationDirection: c.dir,
		})
		if err != nil {
			return made{}, fmt.Errorf("edge to %s: %w", c.to.claim.ID(), err)
		}
		edges = append(edges, edge)
		refs = append(refs, c.to.claim)
	}

	at := s.at
	if at.IsZero() {
		at = g.at
	}
	encoding := s.encoding
	if encoding == "" {
		encoding = "text/plain"
	}
	b := ranke.NewClaim(s.typ, s.by.as).
		WithInlineContent(s.content).
		WithEncoding(encoding).
		WithCreatedAt(at).
		WithHeightResolver(context.Background(), ranke.HeightsFrom(refs...)).
		WithEdges(edges...)
	for key, value := range s.fields {
		b = b.WithField(key, value)
	}
	claim, err := b.Sign()
	if err != nil {
		return made{}, fmt.Errorf("sign %s: %w", s.typ, err)
	}
	// Unscheduled ticks the ambient clock forward; scheduled only ever pulls it ahead.
	if s.at.IsZero() {
		g.at = g.at.Add(clockStep)
	} else if at.After(g.at) {
		g.at = at
	}
	m := made{claim: claim, branch: s.branch}
	g.made = append(g.made, m)
	return m, nil
}

// made pairs a signed claim with the branch holding it — a reference may only reach within
// its own branch's closure.
type made struct {
	claim  ranke.Claim
	branch string
}

// newGrower derives the fixture contributor named by `as` and binds its signing key.
func newGrower(ctx context.Context, as string) (*grower, error) {
	claim, priv, err := identity(as)
	if err != nil {
		return nil, err
	}
	// No Universe: this contributor's pubkey is inline, so nothing needs fetching.
	self, err := claim.AsContributor(ctx, nil, priv)
	if err != nil {
		return nil, fmt.Errorf("bind contributor %q: %w", as, err)
	}
	digest := sha256.Sum256([]byte(as))
	return &grower{
		self:      self,
		selfClaim: claim,
		rnd:       rand.New(rand.NewPCG(uint64(digest[0]), uint64(digest[1]))),
		at:        epoch,
	}, nil
}

// identityKey derives a fixture signing key from a name, so the same name always signs as
// the same identity. The key comes from a public string and is worth nothing: never an
// application's.
func identityKey(as string) (ed25519.PrivateKey, error) {
	seed := sha256.Sum256([]byte(identityDomain + as))
	return ed25519.NewKeyFromSeed(seed[:]), nil
}

// identity derives a root contributor claim from a name: its own key, self-attested.
func identity(as string) (ranke.Claim, ed25519.PrivateKey, error) {
	priv, err := identityKey(as)
	if err != nil {
		return nil, nil, err
	}
	pub, err := ranke.EncodePublicKey(priv.Public())
	if err != nil {
		return nil, nil, fmt.Errorf("encode public key: %w", err)
	}
	// The root contributor claim names no contributor: it is its own (§4.3).
	claim, err := ranke.NewClaim(ranke.NodeTypeContributor, nil).
		WithInlineContent(pub).
		WithEncoding(ranke.EncodingOctetStream).
		WithCreatedAt(epoch).
		Sign(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("sign contributor claim: %w", err)
	}
	return claim, priv, nil
}

// example is the smallest graph with real provenance: two sources, a derivation citing
// both, an entity from that derivation.
func (g *grower) example(branches []string) (batches, error) {
	first, err := g.claim("source/note", "a first note")
	if err != nil {
		return nil, err
	}
	second, err := g.claim("source/note", "a second note")
	if err != nil {
		return nil, err
	}
	derivation, err := g.claim("derivation/extraction", "Ada Lovelace", first, second)
	if err != nil {
		return nil, err
	}
	entity, err := g.claim("entity/person", "Ada Lovelace", derivation)
	if err != nil {
		return nil, err
	}
	out := batches{{
		branch: branches[0],
		claims: []ranke.Claim{first.claim, second.claim, derivation.claim, entity.claim},
	}}
	// A second branch, so an archive has a branch table worth reading and a client has a
	// choice to make. Its own sources, since a reference across branches is a separate
	// question from a branch existing.
	for _, name := range branches[1:] {
		note, err := g.claim("source/note", "a note on "+name)
		if err != nil {
			return nil, err
		}
		summary, err := g.claim("derivation/summary", "what "+name+" says", note)
		if err != nil {
			return nil, err
		}
		out = append(out, batch{branch: name, claims: []ranke.Claim{note.claim, summary.claim}})
	}
	return out, nil
}

// chainTypes is the mix a grown archive gets — sources dominate, as they do in life.
var chainTypes = []string{
	"source/note",
	"source/note",
	"source/record",
	"derivation/extraction",
	"entity/person",
	"relation/mention",
}

// chain grows an archive contribution by contribution, each citing what came before:
// climbing heights, a branch table of many revisions, references that reach back.
func (g *grower) chain(contributions, per int, branches []string) (batches, error) {
	out := make(batches, 0, contributions)
	for i := range contributions {
		// The primary takes every other contribution and the rest share the remainder, so one
		// branch reads as the main line without the others being empty.
		branch := branches[0]
		if i%2 == 1 && len(branches) > 1 {
			branch = branches[1+(i/2)%(len(branches)-1)]
		}
		claims := make([]ranke.Claim, 0, per)
		for j := range per {
			typ := chainTypes[(i*per+j)%len(chainTypes)]
			// A source enters the archive, so it cites nothing; the rest draw on what is there.
			var refs []made
			if !isSource(typ) {
				refs = g.pickFrom(branch, maxReferences)
				// Nothing on this branch to derive from yet, and the provenance invariant
				// (§3.5) requires a derivation edge on every class but source — so the first
				// claims a branch can hold are sources, which is also true of a real archive.
				if len(refs) == 0 {
					typ = chainTypes[0]
				}
			}
			m, err := g.claimOn(branch, typ, fmt.Sprintf("%s %d.%d", typ, i+1, j+1), refs...)
			if err != nil {
				return nil, err
			}
			claims = append(claims, m.claim)
		}
		out = append(out, batch{branch: branch, claims: claims})
	}
	return out, nil
}

// isSource reports whether a type is in the source class, whose claims are roots.
func isSource(typ string) bool {
	return strings.HasPrefix(typ, string(ranke.NodeClassSource)+"/")
}

// How far back "recent" reaches, how often a reference skips that window for anywhere
// in the archive, and how many one claim makes.
const (
	recentWindow  = 40
	farReachOdds  = 8
	maxReferences = 3
)

// pickFrom chooses up to n earlier claims on this branch to cite, biased to the recent and
// occasionally reaching right back. Confined to the branch because a contribution may only
// reference what the branches it declares can reach. Duplicates drop: the same edge twice is
// one edge.
func (g *grower) pickFrom(branch string, n int) []made {
	candidates := make([]made, 0, len(g.made))
	for _, m := range g.made {
		if m.branch == branch {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 || n < 1 {
		return nil
	}
	seen := make(map[string]bool, n)
	refs := make([]made, 0, n)
	for range g.rnd.IntN(n) + 1 {
		i := len(candidates) - 1 - g.rnd.IntN(min(len(candidates), recentWindow))
		if g.rnd.IntN(farReachOdds) == 0 {
			i = g.rnd.IntN(len(candidates))
		}
		if key := candidates[i].claim.ID().String(); !seen[key] {
			seen[key] = true
			refs = append(refs, candidates[i])
		}
	}
	return refs
}

// claim signs one inline-text claim citing refs, and remembers it. Height is explicit
// because a referencing claim must declare it (§4.1), and every claim cites its
// contributor — an initial node at 0, so citing only it sits at 1.
func (g *grower) claim(typ, text string, refs ...made) (made, error) {
	return g.claimOn("", typ, text, refs...)
}

// claimOn signs a claim the grower's own identity attributes, citing refs as inputs — the
// generic shapes need nothing else, so they say it this short way.
func (g *grower) claimOn(branch, typ, text string, refs ...made) (made, error) {
	return g.write(spec{
		by:      g.rootSigner(),
		branch:  branch,
		typ:     typ,
		content: []byte(text),
		cites:   asInputs(refs...),
	})
}
