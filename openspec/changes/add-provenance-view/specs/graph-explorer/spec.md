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

#### Scenario: Several claims' provenance side by side
- **WHEN** a user opens the provenance of a second claim
- **THEN** it opens in its own tab, the first tab keeping its scope, layout and camera

### Requirement: An unread claim is never drawn as an initial claim

The explorer SHALL read the claims a provenance scope names and the session does not hold,
and SHALL distinguish, for every claim it draws, between a claim that references nothing and
a claim whose references it has not read. An initial claim is "a claim with no references; a
root at which provenance traversal terminates", and that a traversal terminates is the
assertion this view exists to make; a claim drawn as initial because the session stopped
there states something the archive does not.

Where a reference was dropped for want of its target, the explorer SHALL restore it once a
later read supplies that target. A read bounded by a result cap or by what a reader has
unfolded leaves unsatisfied references at the edge of what was read, which is exactly where a
provenance view ends, so a merge that never revisits them cannot draw a correct closure.

Until a claim's own references are read, the explorer SHALL present it as unread. Height
determines which case a claim is in — an initial claim carries 0 — so the two SHALL NOT be
distinguished by absence of edges alone.

#### Scenario: A named claim the session lacks is read
- **WHEN** a provenance scope's answer names a claim the session has not read
- **THEN** the explorer reads that claim, rather than leaving it out of the drawing

#### Scenario: A reference dropped earlier is restored
- **WHEN** a claim referencing an absent target was merged, and a later read supplies that target
- **THEN** the reference is drawn, without the citing claim being read again

#### Scenario: An unread claim is marked, not terminated
- **WHEN** a claim in the closure carries a height above zero and its references have not been read
- **THEN** it is drawn as unread, and no claim is drawn as an initial claim unless its height is zero

#### Scenario: A shortfall is stated while it lasts
- **WHEN** a provenance scope names more claims than the session holds
- **THEN** the explorer reports how many are outstanding, as it does for any other scope

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
