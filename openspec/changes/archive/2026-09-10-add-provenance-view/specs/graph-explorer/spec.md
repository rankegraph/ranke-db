## ADDED Requirements

### Requirement: A claim's provenance is browsable as a scope

The explorer SHALL offer the closure of any claim it holds as a view scope, that scope's head
being the claim itself, and SHALL obtain its membership by the identities-only read it uses
for every other scope — `select.head` naming the claim and `output.detail: id` asking for the
ids in its closure. A claim's provenance is a closure and a closure needs a root, which is
the rule that already decides which scopes are browsable; a claim is a root like any other.

A provenance scope SHALL carry the branch it is read within alongside the claim whose closure
it is, since a query names a scope and a head separately, and SHALL be keyed by that head
wherever membership is recorded, so two claims' provenance never share one answer.

The explorer SHALL NOT compute the closure itself. Reachability is the query engine's work,
here for the same reason it is for a branch: a client that walked closures would be a second
query engine, in the layer furthest from the data.

#### Scenario: A claim's closure is asked for by head
- **WHEN** a user opens the provenance of a claim
- **THEN** the explorer asks its data source for the ids in the closure of that claim, naming the claim as the head and the branch it was reached through as the scope, and computes no closure of its own

#### Scenario: Two claims' provenance are two answers
- **WHEN** the provenance of two different claims is open at once
- **THEN** each view is confined to its own id set, neither overwriting the other

#### Scenario: A shared ancestor appears in both
- **WHEN** two claims' closures both contain a third claim
- **THEN** each view draws it, the store holding one node for it

### Requirement: A provenance view is an ordinary graph view

The explorer SHALL draw a claim's provenance with the canvas, layouts, camera, panes and
lens it draws every other view with, and SHALL introduce no second rendering of a graph. A
provenance view is a view over the one union graph confined to a scope, which is what every
view is; a drawing of its own would be a second answer to a question already answered.

Opening the provenance of a claim SHALL open a tab for that claim, and asking again for a
claim whose tab is open SHALL bring that tab forward rather than adding a second — a claim is
content-addressed, so two tabs would draw the same closure twice.

#### Scenario: The same tools draw it
- **WHEN** a provenance view is active
- **THEN** the layouts, the camera bound, the selection panes and the lens behave as they do in any other graph view

#### Scenario: One tab per claim
- **WHEN** a user asks twice for the provenance of the same claim
- **THEN** the open tab comes forward, and one tab exists for that claim

#### Scenario: The layout a view names is the layout it draws
- **WHEN** a provenance view opens and names its layout
- **THEN** that layout has been applied, rather than the coordinates a previous read left behind

#### Scenario: Several claims' provenance side by side
- **WHEN** a user opens the provenance of a second claim
- **THEN** it opens in its own tab, the first tab keeping its scope, layout and camera

### Requirement: A provenance view keeps the scope it was opened at

A provenance view's scope SHALL be fixed when the view is opened, and the branch picker SHALL
NOT retarget it. A provenance view is the view of one claim's closure — that is what its tab
is named for and keyed on — so a scope chosen elsewhere replacing it would leave a view still
named for a claim while drawing something else.

Choosing a branch while a provenance view is active SHALL therefore change what the next read
is scoped to, and SHALL leave every open provenance view drawing the closure it was opened at.
A user who wants a branch view SHALL open one, the explorer already drawing each scope in its
own view.

The picker SHALL report the active view's scope rather than a scope that view is not confined
to. Where the active view is a provenance view, the picker offering only the discovered
branches has no entry that describes it, and SHALL say so rather than display whichever branch
happens to match nothing.

#### Scenario: A branch chosen elsewhere leaves provenance intact
- **WHEN** a provenance view is active and a user picks a branch in the scope picker
- **THEN** the view goes on drawing that claim's closure, and the branch applies to the next read

#### Scenario: The picker does not misreport a provenance view
- **WHEN** a provenance view is active
- **THEN** the picker presents that view's scope as the provenance it is, rather than naming a branch the view is not confined to

#### Scenario: A branch view is still retargeted
- **WHEN** a branch-scoped view is active and a user picks a different branch
- **THEN** that view is confined to the newly chosen branch, as it is today

### Requirement: An unread claim is never drawn as an initial claim

The explorer SHALL read the claims a provenance scope names and the session does not hold,
and SHALL distinguish, for every claim it draws, between a claim that references nothing and
a claim whose references it has not read. An initial claim is "a claim with no references; a
root at which provenance traversal terminates", and that a traversal terminates is the
assertion this view exists to make; a claim drawn as initial because the session stopped
there states something the archive does not.

Until a claim's own references are read, the explorer SHALL present it as unread. Height
determines which case a claim is in — an initial claim carries 0 — so the two SHALL NOT be
distinguished by absence of edges alone.

#### Scenario: A named claim the session lacks is read
- **WHEN** a provenance scope's answer names a claim the session has not read
- **THEN** the explorer reads that claim, rather than leaving it out of the drawing

#### Scenario: An unread claim is marked, not terminated
- **WHEN** a claim in the closure carries a height above zero and its references have not been read
- **THEN** it is drawn as unread, and no claim is drawn as an initial claim unless its height is zero

#### Scenario: A shortfall is stated while it lasts
- **WHEN** a provenance scope names more claims than the session holds
- **THEN** the explorer reports how many are outstanding, as it does for any other scope

### Requirement: A reference dropped for want of its target is re-added when the target arrives

The explorer SHALL merge a claim's reference into the graph as soon as both ends are present,
including when the target arrives in a later read than the reference did. A reference whose
target was absent at merge time SHALL be retained and retried rather than discarded, and the
number still outstanding SHALL be reported rather than counted silently.

A read bounded by a result cap or by what a reader has unfolded leaves unsatisfied references
at the edge of what was read, which is exactly where a provenance view ends — so a merge that
never revisits them cannot draw a correct closure, and the claim that stated such a reference
is drawn as an initial claim, which is the one thing this view asserts.

#### Scenario: A reference dropped earlier is restored
- **WHEN** a claim referencing an absent target was merged, and a later read supplies that target
- **THEN** the reference is drawn, without the referencing claim being read again

#### Scenario: An outstanding reference is not silently absent
- **WHEN** references remain whose targets have not been read
- **THEN** the explorer reports how many, rather than discarding the count

### Requirement: A view already drawn is not disturbed by a later read

A later read SHALL NOT move what a reader is looking at: the camera SHALL keep its framing,
the lens its window, and the selection its claim. The claims that arrive SHALL be laid out, so
none is drawn at the origin for want of a position.

Positions are a property of the one shared union graph, so a layout is union-wide and a claim
already drawn may be placed differently once more of the graph is known. That is what a shared
graph means, and an ordinary read already does it; what a later read must not do is reframe,
so the picture stays where the reader put it even as it fills in.

#### Scenario: A merged claim is placed, not left at the origin
- **WHEN** further claims of a provenance scope are merged into an open view
- **THEN** each is laid out by that view's layout, rather than drawn at the coordinate an unplaced node carries

#### Scenario: An incremental read leaves the reading in place
- **WHEN** further claims of a provenance scope are read into an open view
- **THEN** the camera keeps its framing, the lens its window and the selection its claim

#### Scenario: The view draws what is held before the rest arrives
- **WHEN** a provenance scope's identities are answered and some of the claims are not yet read
- **THEN** the view draws the claims the session holds, rather than waiting for the whole closure

#### Scenario: Another open view is left alone
- **WHEN** a provenance read merges claims into the shared union graph
- **THEN** a view open elsewhere keeps its camera and layout, rather than being reframed by claims it did not ask for

### Requirement: The explorer names the two directions as the glossary does

The explorer SHALL name what a claim points at a **reference**, the set reachable from a
claim its **closure**, a claim with no references an **initial claim**, and SHALL NOT use
*cite* or *citation* for either direction. The glossary fixes the first three and defines no
citation, which is how one word came to carry opposite senses in one pane: the past in "what
this claim cites" and the future in "what cites this claim".

#### Scenario: A pane names a direction once
- **WHEN** a pane names what a claim points at, or what points at it
- **THEN** it does so as *references* and *referenced by*, and the word *cite* appears in neither

#### Scenario: A drawn terminus is named for what it is
- **WHEN** a view draws a claim at which traversal terminates
- **THEN** it names it an initial claim, the term the glossary fixes

## MODIFIED Requirements

### Requirement: The selectable scopes are those that have a head

The explorer SHALL offer as a view scope the whole archive (`$archive`, the closure of
the archive head), each named branch (the closure of its own head), and the provenance of a
claim (the closure of that claim), and SHALL NOT offer `$universe`. A scope is browsable only
if it has a head its closure is rooted at; `$universe` has none, which is why RankeQL requires
an explicit head to read there at all. It remains reachable as a by-id read, not as a scope a
view can be confined to.

The three are reached from different places, since they are chosen from different things. The
archive and the branches are what discovery returns, so the scope picker offers them; a claim's
provenance is chosen by choosing a claim, so it is reached from that claim. The picker
therefore SHALL NOT enumerate provenance scopes — there is one per claim in the archive.

A scope SHALL carry the branch it is read within beside the head whose closure it is. For a
branch scope the two agree — the branch's own name and its own head; for the archive it is the
archive-wide scope and the branch-table head; for a provenance scope it is the branch the claim
was reached through and the claim itself.

#### Scenario: The archive is selectable alongside the branches
- **WHEN** a user opens the scope picker
- **THEN** it offers the whole archive and each named branch, every option carrying a head

#### Scenario: The Universe is not a browsable scope
- **WHEN** the scope picker is inspected
- **THEN** `$universe` is absent, having no head and so no closure to name

#### Scenario: A provenance scope is a scope by the same rule
- **WHEN** a claim's provenance is opened as a scope
- **THEN** it is browsable because it has a head — the claim — and carries the branch it is read within

#### Scenario: Provenance is reached from a claim, not from the picker
- **WHEN** a user opens the scope picker
- **THEN** it lists no provenance scope, those being chosen by choosing a claim rather than from a listing

### Requirement: Membership is the engine's answer, not the client's

The explorer SHALL obtain a scope's membership by asking its data source a scoped query for
identities only — `select.branch` naming the branch the scope is read within, `select.head`
naming the head whose closure it is, and `output.detail: id` asking for the ids in it — and
SHALL NOT traverse the graph itself to decide what a scope contains. Membership is reachability
from that scope's head, and computing it is the query engine's work: a client that walked
closures would be a second query engine, in the layer furthest from the data and least able to
optimise it.

The explorer SHALL then draw the intersection of that answer with the claims it holds, so
switching scopes costs a query for identities and a lookup per claim rather than re-reading
any claim body.

A remembered answer SHALL be keyed by the head whose closure it is, not by the scope's name. A
head identifies a closure and a name does not: two provenance scopes read within one branch
share that branch's name, so keying by name would give them one another's membership.

A read bounded by a result cap SHALL NOT be recorded as a scope's membership. Such a read
returns part of the closure, and remembering it as the whole would confine the view to a
boundary the archive does not have; the scope is left unasked, which already admits everything.

It SHALL NOT record a single owning scope on a claim: a claim reached from several branches
belongs to all of them and is one node in the store, so membership is a set the answer
defines and never a label the claim carries.

A scope the explorer has not yet asked about SHALL admit everything, so a view is never
blank for want of an answer.

#### Scenario: The engine decides what a branch contains
- **WHEN** a user selects a branch
- **THEN** the explorer asks the source for the identities in that scope and confines the view to them, computing no closure of its own

#### Scenario: A shared claim appears under both branches
- **WHEN** a claim is named by the answers for two branches
- **THEN** selecting either draws it, the store holding one node for it

#### Scenario: An answer already held is not asked for again
- **WHEN** a branch is selected, deselected and selected again
- **THEN** the identities already returned for it are reused

#### Scenario: An unanswered scope hides nothing
- **WHEN** a view names a scope whose identities have not been returned
- **THEN** it admits every claim rather than drawing empty

#### Scenario: Two provenance scopes keep separate answers
- **WHEN** the provenance of two claims on one branch is read
- **THEN** each scope holds its own membership, neither overwriting nor reading the other's

#### Scenario: A capped read is not remembered as a closure
- **WHEN** a read of a scope stops at its result cap
- **THEN** the claims it returned are not recorded as that scope's membership
