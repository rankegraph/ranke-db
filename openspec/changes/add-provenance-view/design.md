## Context

A claim's provenance is its closure: "the transitive set of claims reachable from a claim by
following its edges to their references" (glossary, §closure), terminating at initial claims
— "a claim with no references; a root at which provenance traversal terminates" (§initial
claim). Height states the same structure numerically, "the longest path from the claim
following references, an initial claim carrying 0" (§height), and is signed into every claim.

The explorer holds one union graph, draws it through views that are reducer selections over
that graph, and confines a view to a scope by asking the engine which claims the scope
contains. Everything a provenance view needs is therefore in place except the scope itself.

## Goals / Non-Goals

**Goals.** A reader looking at a claim can open its provenance and read the closure as a
picture, in a tab of its own, several claims at once. The closure comes from the engine. The
view never claims a derivation ends where the session merely stops.

**Non-Goals.** No second rendering. No client-side closure walk. No new port method, and no
change to the contract, the engine or `ranke-go`. The other direction — the claims that
reference this one, which the query language calls `uses` — is out of scope: provenance is
the past.

## Decisions

### The claim is the head, not an anchor

RankeQL offers two ways to name a closure. `select.claim` anchors a frontier and, with no
`path`, "returns the full outward closure of the frontier" (`R-QSTEPS`); `head` "fixes the
closure read: the query sees the intersection of the scope's graph and closure(head)"
(`R-QHEAD`). Both answer with the same set. The head is chosen because it is the shape the
explorer already sends and the shape a scope already is: `Scope` is `{ name, head }`, and
`scopeIds` builds `{select: {branch, head}}` today (`source.ts:249`). Reading a claim's
provenance is then the read that exists, given a different head, and the whole feature
inherits scope confinement, shortfall reporting and the view predicate for free.

`R-QSCOPE` still requires a real scope name on the wire — a branch, `$archive` or
`$universe` — so a provenance scope must carry both: the branch it is read within, and the
claim whose closure it is. That is the one field `Scope` lacks.

### It is a graph view, not a document

A provenance closure could be drawn as a document: rows, indented, down to the initial
claims, the way the CBOR tab renders a parsed record. It is drawn on the canvas instead,
because the canvas is what the explorer is, and because a closure is a DAG rather than a
tree — a document would either repeat a claim reached by two paths or hide one of the paths,
and both misstate the structure. `layered` already lays claims out by provenance depth
(`layouts.ts:188`, depths from `graph/shape.ts:52`), so the layout is written.

The cost of this choice is that a claim can only be drawn once it is a node in the union
graph, which is what makes the lazy read and the dropped-reference repair part of the work
rather than refinements of it.

### The closure is fetched as identities, and claims are read as needed

`graph-explorer` already requires that membership be "the engine's answer, not the client's",
on the ground that "a client that walked closures would be a second query engine, in the
layer furthest from the data and least able to optimise it". A provenance closure is the same
question with a different root, so it is answered the same way: `output.detail: id`, and the
view draws the intersection with what the session holds.

Claims the answer names and the session lacks are then read, rather than reported and left
out. A branch scope can report a shortfall and still be useful, because a branch view is a
sample of a branch. A provenance view is a statement about one claim's derivation, and a
missing claim in the middle of it makes the picture wrong rather than partial.

### A reference is re-added when its target arrives

`addEdges` iterates the claims of the page being merged and drops any edge whose target the
graph does not hold (`build.ts:337-340`). With one modal `load` per session that was a
tolerable simplification. With a lazy read it is a correctness bug, and the sharpest one
here: the references dropped are precisely those pointing past the edge of what was loaded,
which is exactly where a provenance view ends, and a claim whose references were dropped is
indistinguishable on the canvas from an initial claim.

So a merge must revisit the references it could not satisfy. Two ways: keep the unsatisfied
references as pending and drain them on each merge, or re-walk the citing claims after a
merge that added nodes. The first is bounded by what was actually dropped and is preferred;
either way `danglingRefs`, computed and discarded today (`build.ts:169`, `universe.ts:28`),
becomes a number the view reads.

### Unread and initial are drawn apart

A claim in the closure whose own references have not been read yet is drawn as unread. An
initial claim is drawn as initial. Height gives the check for free: an initial claim carries
0, so a claim drawn with no references and a height above 0 is a claim the session has not
finished reading, and that is a bug the view can detect rather than a state it can only hope
to avoid.

### The closure is not filtered by edge class

A claim's closure is everything reachable from it. Filtering to `derivation/*` and
`contribution/*` — the two edge classes the foundation paper calls provenance edges and
contribution edges — would drop a relation's participants, which are reachable and therefore
part of the closure. `R-QSTEPS`'s `edges` glob makes such a filter available later as a view
option; it is not what provenance means.

## Risks / Trade-offs

- **A head claim's closure is the branch.** Anchoring on a `contribution/head` claim asks for
  everything below it, and on a branch table, the archive. The id set is cheap and the count
  is honest, so the view can say what it is about to draw; how much of it to read is bounded
  by the same result cap every other read has.
- **Lazy reads are one claim at a time.** `claimBytes` reads a single claim
  (`source.ts:78`), so filling a large closure is many requests. Acceptable for a first cut,
  since a reader unfolds a closure rather than demanding all of it; a batched read is an
  upstream ask if it becomes the bottleneck.
- **The incremental merge touches the shared graph.** A provenance tab's read adds claims
  every other view can then draw. That is what a union graph means, and it is the same
  behaviour two branch loads already have — but it does mean a provenance read widens the
  time axis, and the timeline layout takes its extent from every instant loaded
  (`core/timeline.ts:62`). The merge must therefore leave an open view's camera and layout
  alone, or reading provenance moves the picture a reader was looking at.

## Resolved while planning

- **Whether provenance runs to the future.** It does not. Provenance is the past: the closure
  of the claim. What references a claim is the query language's `uses` direction and a
  separate question, worth its own switch once this exists.
- **Whether the tab needs a new tab kind in the store.** No — drawing it as a graph view
  means it is a `ViewState` with a provenance scope, so the tab strip, the render switch and
  `activeView` need nothing (`store.ts:67`, `App.tsx:353,365`).
- **Whether the explorer should build the closure from the graph it already holds.** No. The
  union graph is a non-multi `DirectedGraph` merged with `mergeDirectedEdge`
  (`build.ts:342`), so two references of different classes between the same pair of claims
  collapse into one, and dropped references are missing outright. The graph is a cache of
  claims, not a copy of the archive's structure.
