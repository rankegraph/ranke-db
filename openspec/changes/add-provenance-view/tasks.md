## 1. A scope can be a claim's closure

- [ ] 1.1 Give `Scope` the branch it is read within beside its head (`core/scope.ts:18`), since `R-QSCOPE` requires a scope name on the wire and a provenance scope's head is a claim rather than a branch head. Every existing scope keeps its own name as that branch.
- [ ] 1.2 Add a constructor for a claim's provenance scope and a label for it, so the label rule stays where the vocabulary lives (`scopeLabel`, `core/scope.ts:30`).
- [ ] 1.3 Key membership by head rather than by scope name (`core/graph/members.ts:15`), so two provenance scopes hold two id sets. Check every caller: `session.ts:161-163` writes a scope's members once, `session.ts:303` replaces them on selection.
- [ ] 1.4 Send the scope's own branch and head from `scopeIds` (`core/data/source.ts:249`). No new port method, no `select.claim`: the read is the one that exists with a different head.
- [ ] 1.5 Answer the same read in `MockSource`, whose generated archive it owns, so both backends answer one question two ways.
- [ ] 1.6 Confirm `inScope` (`core/session.ts:339`) needs no change — a provenance view is confined by the same predicate as a branch view.

## 2. The provenance tab

- [ ] 2.1 Add `openClaimProvenance(id)` beside `openClaimCbor` (`core/session.ts:520`): dedupe on the claim id, bring an open tab forward, otherwise add a view whose scope is that claim's provenance and whose layout is `layered`.
- [ ] 2.2 Add the `show claim provenance` action to the selection pane beside `show claim CBOR` (`ui/panes/SelectionPane.tsx:169`).
- [ ] 2.3 Confirm nothing in the tab strip or the render switch changes, the tab being a `ViewState` (`core/store.ts:67`, `ui/shell/App.tsx:353,365`). If either needs a case, the tab is not a view and 2.1 is wrong.
- [ ] 2.4 Label the tab by the claim, so several open tabs are told apart by what they are about rather than by number.

## 3. Reading what the session lacks

- [ ] 3.1 Add an incremental merge the session can call outside `load` — `mergeClaims` exists on the graph (`core/graph/universe.ts:46`) and only `load` calls it through `mergeClaimsProgressively`. It must not patch the active view's classes, relayout the union, rebuild the time axis, reframe the camera, drop the lens or raise `status.busy`, all of which `load` does (`core/session.ts:74-204`).
- [ ] 3.2 Read a claim the closure names and the session lacks, decoding it with the ADT reader — `claimBytes` (`core/data/source.ts:78`) already reads one claim's signed bytes and the CBOR tab already caches them, so a claim read for either is read once.
- [ ] 3.3 Lay out only what arrived, and leave an open view's camera where the reader put it. A timeline view takes its extent from every instant loaded (`core/timeline.ts:62`), so a read that widens the axis must be handled deliberately rather than by a full reframe.
- [ ] 3.4 Report the outstanding count while a closure is incompletely read, through the shortfall the spec already requires.

## 4. References dropped for want of a target

- [ ] 4.1 Hold the references `addEdges` cannot satisfy (`core/graph/build.ts:337-340`) instead of discarding them, and drain them on each merge that adds nodes. Bound the held set by what was actually dropped.
- [ ] 4.2 Surface `danglingRefs` — computed and read nowhere today (`build.ts:169`, `core/graph/universe.ts:28`) — so an incomplete closure is a number a view can state.
- [ ] 4.3 Draw a claim whose references are unread as unread, and a claim of height 0 as an initial claim. The two must not be distinguished by absence of edges alone.
- [ ] 4.4 Decide what a parallel reference does to the drawing: the union is a non-multi graph and `mergeDirectedEdge` (`build.ts:342`) collapses two references of different classes between one pair of claims into one edge. Either accept it and say so, or hold the claim's own edge list where the count matters.

## 5. Vocabulary

- [x] 5.1 Replace *cite* and *citation* with *reference* and *referenced by* in the explorer's panes and comments, and name an edge as the glossary does — "a typed link from the claim that owns it to the claim it references". Applied ahead of the rest: `SelectionPane.tsx` (four), `InfoPane.tsx` (three), `renderer.ts` (two), `lens.ts`, `build.ts`, `lens.test.ts` (two), `mock/generate.ts` (five), and on the Go side `contribute_test.go`, `endpoints_scope_test.go`, `contribute.go`, `branch/create.go` (two), `graph_test.go`, `errors_test.go`, plus one scenario title in `core-contribution/spec.md`.
- [ ] 5.2 Rename `detail.citedBy` and `detail.citations` (`core/session.ts:678`, `claimDetail`), the last of the word in the explorer, and the two tests that name them.

## 6. Tests

- [ ] 6.1 A provenance scope's read names the claim as the head and the branch as the scope, against both sources.
- [ ] 6.2 Two provenance scopes hold two id sets, and a claim in both is one node.
- [ ] 6.3 A second read adds claims without moving an open view's camera, changing its classes or relaying out the union.
- [ ] 6.4 A reference dropped in one merge is drawn after a later merge supplies its target.
- [ ] 6.5 A claim of height above zero whose references are unread is not drawn as an initial claim.
- [ ] 6.6 Asking twice for one claim's provenance leaves one tab, and it carries no field a graph view does not have.

## 7. Documentation

- [ ] 7.1 A changelog entry under `## Unreleased`, naming the action a reader gains and the scope kind it opens.
- [ ] 7.2 `frontend/README.md`, where the views and tabs are described.
- [ ] 7.3 Correct `CLAUDE.md`'s stale anchor while nearby: out-of-scope claims are omitted from a read ("claims outside the scope's closure simply do not appear", `openapi/openapi.yaml:188`), rather than returned as hash-only stubs; what keeps a returned subgraph hashable is the reference id inside each claim's own envelope bytes. The word *stub* appears in neither the spec nor the papers.
