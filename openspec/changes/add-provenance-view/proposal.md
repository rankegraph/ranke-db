## Why

Provenance is what the Ranke-Graph is for. The foundation paper reserves the word for "the
chain of derivation back to sources and contributors", and its §Provenance states the
property a reader would be looking at: reference traversal from any claim is acyclic and
finite, terminating at an initial claim per path, so a claim's provenance is its closure and
asking for it is O(n).

The explorer cannot show it. A view is confined to a branch or to the whole archive — the
scopes discovery returns — and a reader asking what one claim rests on gets the reference
list in the selection pane, one hop deep, and walks the rest by hand, a click and a pane
read per claim. The graph tab draws every claim the session holds, which answers a different
question. So the one structure the archive exists to preserve is the one thing the explorer
has no view of.

Nothing new has to be built to show it. A scope in the explorer is already `{ name, head }`,
browsable exactly when it has a head, "because a view predicate is a closure test and a
closure needs a root" (`frontend/src/core/scope.ts:18`). A claim's provenance is a closure
whose root is that claim. `R-QHEAD` says `head` fixes the closure read, so the query is the
one `scopeIds` already sends — `{select: {branch, head}}` with `output.detail: id`
(`frontend/src/core/data/source.ts:249`) — with the claim's id in place of a branch head.
The engine answers, as it already does for branch membership, and the view predicate that
confines a branch view confines this one (`inScope`, `session.ts:339`).

What the change costs is therefore not a renderer. It is three things the existing scope path
does not survive: membership keyed by scope *name* rather than by head, a session that can
read claims only through the modal `load`, and a merge that drops a reference whose target is
absent and never revisits it. That last one is why this cannot be shipped as a filter alone:
a dropped reference draws a claim as an initial claim, and an initial claim is the one thing
this view asserts.

## What Changes

- **A claim's provenance becomes a scope.** `Scope` gains the branch a scope is read within
  (`select.branch`, `R-QSCOPE`) beside the head whose closure it is, so a provenance scope
  carries the claim as its head and the branch it was reached through as its scope. The
  existing identities-only read answers it unchanged.
- **Membership keys on the head.** `setMembers` keys an id set by scope name today
  (`frontend/src/core/graph/members.ts:15`), so two provenance scopes would share one set.
  The key becomes the head, which is unique per closure and already what a scope means.
- **A provenance tab is a graph view.** One tab per claim, opened from the selection pane
  beside `show claim CBOR` and keyed on the claim id so a second request brings the open tab
  forward. It draws with the canvas, layouts, camera, panes and lens every other view uses;
  `layered` (`core/layout/layouts.ts:188`) is the layout that reads the closure as what it
  is, layer by layer.
- **The claims a provenance scope names are read lazily.** The id set arrives first and the
  view draws the claims the session holds; the rest are read as needed. Today `load`
  (`core/session.ts:74`) is the only way a claim reaches the store, and it patches the active
  view's classes, relayouts every node, rebuilds the time axis from every instant, reframes
  the camera, drops the lens and covers the canvas. A second read needs a path that adds
  claims without any of that.
- **A reference dropped for want of its target is re-added when the target arrives.**
  `addEdges` skips such an edge and counts it in `danglingRefs`
  (`core/graph/build.ts:337-340`), which nothing reads. A lazy read makes this a correctness
  bug rather than a statistic: the reference that was dropped is exactly the one at the edge
  of what had been loaded, and its absence reads as *initial claim*.
- **A claim whose own references are unread says so.** Until its references are in hand a
  claim is drawn as unread, never as an initial claim — the distinction between "this is
  where the derivation ends" and "this is where the session ends" is the whole reading.
- **`cite` leaves the explorer's vocabulary.** The glossary defines *reference* as "the claim
  an edge points at — the target of provenance traversal" and has no entry for *citation*, so
  the word carried opposite senses in the same pane: "What this claim cites" for the past and
  "What cites this claim" for the future. Applied ahead of the rest of this change, since a
  provenance view would spread it further.

## Capabilities

### Modified Capabilities

- `graph-explorer`: gains provenance as a browsable scope, the rule that a provenance view is
  an ordinary graph view, the guarantee that an unread claim is never presented as an initial
  claim, and the vocabulary the glossary fixes.

## Impact

- **`frontend/src/core/scope.ts`**: `Scope` carries the branch it is read within beside its
  head; a constructor for a claim's provenance scope and a label for it.
- **`frontend/src/core/graph/members.ts`**: keyed by head.
- **`frontend/src/core/data/source.ts`**: `scopeIds` sends the scope's own branch and head,
  which is the shape it already builds. No new port method, no anchored query, no
  `select.claim`.
- **`frontend/src/core/session.ts`**: a provenance scope's read and an incremental claim read
  that does not disturb an open view; `openClaimProvenance` beside `openClaimCbor:520`.
- **`frontend/src/core/graph/build.ts`**: dropped references retried on a later merge, and
  the count surfaced rather than discarded.
- **`frontend/src/core/graph/universe.ts`**: an incremental merge the session can call
  outside `load`.
- **`frontend/src/ui/panes/SelectionPane.tsx`**: the `show claim provenance` action.
- **`frontend/src/ui/shell/App.tsx`**: nothing, if the tab is a view — which is the point of
  drawing it as one.
- **Not affected**: the server, the contract, `ranke-go`. Every read this change makes is one
  the contract already serves and the engine already answers; what is missing is a caller.
