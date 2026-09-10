## 1. A scope can be a claim's closure

- [x] 1.1 Give `Scope` the branch it is read within beside its head (`core/scope.ts:18`), since `R-QSCOPE` requires a scope name on the wire and a provenance scope's head is a claim rather than a branch head. Every existing scope keeps its own name as that branch.
- [x] 1.2 Add a constructor for a claim's provenance scope and a label for it, so the label rule stays where the vocabulary lives (`scopeLabel`, `core/scope.ts:30`).
- [x] 1.3 Key membership by head rather than by scope name (`core/graph/members.ts:15`), so two provenance scopes hold two id sets. Check every caller: `session.ts:161-163` writes a scope's members once, `session.ts:303` replaces them on selection.
- [x] 1.4 Send the scope's own branch and head from `scopeIds` (`core/data/source.ts:249`). No new port method, no `select.claim`: the read is the one that exists with a different head.
- [x] 1.5 Answer the same read in `MockSource`, whose generated archive it owns, so both backends answer one question two ways.
- [x] 1.6 Confirm `inScope` (`core/session.ts:339`) needs no change — a provenance view is confined by the same predicate as a branch view.
- [x] 1.7 Guard `load`'s "the claims that came back are the membership" shortcut (`session.ts:161-163`): where the read hit its result cap it is a truncated answer, and recording it as the closure would make the view lie. Leave membership unasked there, which already admits everything.

## 2. The provenance tab

- [x] 2.1 Add `openClaimProvenance(id)` beside `openClaimCbor` (`core/session.ts:520`): dedupe on the claim id, bring an open tab forward, otherwise add a view whose scope is that claim's provenance and whose layout is `layered`.
- [x] 2.2 Add the `show claim provenance` action to the selection pane beside `show claim CBOR` (`ui/panes/SelectionPane.tsx:169`).
- [x] 2.3 Confirm nothing in the tab strip or the render switch changes, the tab being a `ViewState` (`core/store.ts:67`, `ui/shell/App.tsx:353,365`). If either needs a case, the tab is not a view and 2.1 is wrong.
- [x] 2.4 Label the tab by the claim, so several open tabs are told apart by what they are about rather than by number.

## 3. Reading what the session lacks

- [x] 3.1 Add an incremental merge the session can call outside `load` — `mergeClaims` exists on the graph (`core/graph/universe.ts:46`) and only `load` calls it through `mergeClaimsProgressively`. It must not patch the active view's classes, relayout the union, rebuild the time axis, reframe the camera, drop the lens or raise `status.busy`, all of which `load` does (`core/session.ts:74-204`).
- [x] 3.2 Read a claim the closure names and the session lacks. Done as `claimAt` on the port — `select.claim` anchors the frontier and an empty `path` walks nowhere, so one claim answers. Used below `BY_ID_BELOW`; above it one scoped read is cheaper than a request apiece.
- [x] 3.3 Lay out what arrived and leave the camera where the reader put it. Positions are attributes of the one shared graph, so the layout is union-wide and the framing is the caller's choice: `layOut(layout, stretch, 'keep')`, where `relayout` passes `'fit'`.
- [x] 3.4 Report the outstanding count while a closure is incompletely read, through the shortfall the spec already requires.

## 4. References dropped for want of a target

- [x] 4.1 Hold the references `addEdges` cannot satisfy (`core/graph/build.ts:337-340`) instead of discarding them, keyed by the target id they wait on, and drain them on each merge that adds nodes. Bound the held set by what was actually dropped.
- [x] 4.2 Surface `danglingRefs` — computed and read nowhere today (`build.ts:169`, `core/graph/universe.ts:28`) — so an incomplete closure is a number a view can state. It now means "references whose target is unread" rather than "references discarded".
- [x] 4.3 Draw a claim whose references are unread as unread, and a claim of height 0 as an initial claim. The two must not be distinguished by absence of edges alone.
- [ ] 4.4 Decide what a parallel reference does to the drawing: the union is a non-multi graph and `mergeDirectedEdge` (`build.ts:342`) collapses two references of different classes between one pair of claims into one edge. Either accept it and say so, or hold the claim's own edge list where the count matters.

## 5. The picker leaves a provenance view alone

- [x] 5.1 `selectScope` patches the active view's scope unconditionally (`core/session.ts:253`) and the header picker calls it on every change (`ui/shell/Header.tsx:134`), so a branch pick silently converts an open provenance view into a branch view still named for the claim. Retarget the active view only where it is branch-scoped, and set the session's branch either way — that is what the next read is generated from.
- [x] 5.2 `scopeOptions` (`core/scope.ts:53`) builds from the discovered branches alone, which a provenance scope is never among, so `value={selected}` matches no option at all. Describe the active view's scope where it is not one of the listed branches, so the picker never names a branch the view is not confined to.

## 6. Vocabulary

- [x] 6.1 Replace *cite* and *citation* with *reference* and *referenced by* in the explorer's panes and comments, and name an edge as the glossary does — "a typed link from the claim that owns it to the claim it references". Applied ahead of the rest: `SelectionPane.tsx` (four), `InfoPane.tsx` (three), `renderer.ts` (two), `lens.ts`, `build.ts`, `lens.test.ts` (two), `mock/generate.ts` (five), and on the Go side `contribute_test.go`, `endpoints_scope_test.go`, `contribute.go`, `branch/create.go` (two), `graph_test.go`, `errors_test.go`, plus one scenario title in `core-contribution/spec.md`.
- [x] 6.2 Rename `detail.citedBy` and `detail.citations` (`core/session.ts:677-678`, `claimDetail`), the last of the word in the explorer, and the two tests that name them.

## 7. Tests

- [x] 7.1 A provenance scope's read names the claim as the head and the branch as the scope, against both sources.
- [x] 7.2 Two provenance scopes hold two id sets, and a claim in both is one node.
- [x] 7.3 A second read adds claims without moving an open view's camera, changing its classes or relaying out the union.
- [x] 7.4 A reference dropped in one merge is drawn after a later merge supplies its target.
- [x] 7.5 A claim of height above zero whose references are unread is not drawn as an initial claim.
- [x] 7.6 Asking twice for one claim's provenance leaves one tab, and it carries no field a graph view does not have.
- [x] 7.7 Picking a branch with a provenance view active leaves that view's scope and drawing unchanged, and still retargets a branch-scoped view.
- [x] 7.8 A capped read does not record its claims as a scope's membership.

## 8. Documentation

- [x] 8.1 A changelog entry under `## Unreleased`, naming the action a reader gains and the scope kind it opens.
- [x] 8.2 `frontend/README.md`, where the views and tabs are described.
- [x] 8.3 Correct `CLAUDE.md`'s stale anchor while nearby: out-of-scope claims are omitted from a read ("claims outside the scope's closure simply do not appear", `openapi/openapi.yaml:188`), rather than returned as hash-only stubs; what keeps a returned subgraph hashable is the reference id inside each claim's own envelope bytes. The word *stub* appears in neither the spec nor the papers.

## 9. Verify

- [x] 9.1 `make -C frontend test` and `make -C frontend check` green.
- [ ] 9.2 `make verify` green.
- [~] 9.3 Exercise against a seeded instance. Done at the API level (`bin/ranke-db run --dev`, seeded `generator example`): the provenance read `{select:{branch,head:<claim>}}` answers a 5-of-8 closure containing the claim, and the claim route serves the bytes `claimAt` decodes. Also settled that an empty `select.path` answers as an absent one — the whole closure — which is why `claimAt` reads the claim route rather than an anchored query. NOT done in a browser: the drawing, the unread marking and the late-target repair are unverified on screen.
