# Ranke Explorer

A pure browser client for a RankeDB archive: a static bundle with no application
server, no proxy and no database of its own. It talks straight to a ranke-db REST
endpoint, can hold several instances at once, and works with none at all against
generated mock data.

```sh
make dev       # dev server — open the URL, then press 'load' in the header
make           # build dist/explorer.html, the self-contained distributable
make bench     # the headless performance numbers below
make help      # the rest
```

Dependencies install themselves on first use, so a clean checkout needs only node.

`dist/explorer.html` is committed and self-contained — open it with no npm and no server.

## Architecture

Strict one-way layering; the boundary is the point, not a preference.

| Layer | Holds | Never |
|---|---|---|
| `src/core/` | the store (zustand), the union graph, layouts, shape statistics, mock data, connections | React, DOM, Sigma |
| `src/render/` | the one Sigma instance, at module scope, and the reducers a view is expressed through | graph mutation on a view switch |
| `src/ui/` | React components that read state and dispatch actions | graph data in state, context or props |

Two rules follow from measurement rather than taste:

- **One store holds the union; views are selections over it.** A claim's id is its
  content address, so a claim reached by two queries merges to one node with no
  reconciliation. Switching a view is a reducer swap, never a rebuild.
- **The renderer lives outside React.** Sigma is constructed once for the canvas host
  and is never torn down by a re-render.

### Nothing here declares the model or the contract

Two dependencies own what this client used to restate, and the boundary is as strict as the
layering above:

| From | Comes | So this repo declares |
|---|---|---|
| `@rankegraph/ranke` | `Claim`, `Edge`, the node classes, the content declaration, the sequence readers (claims, ids, raw records), the type-glob matcher, the RankeQL `Query` | no claim type, no vocabulary, no framing rule |
| `openapi/openapi.gen.ts` | every route, every request and response type | no path string, no wire field name |

What the explorer *does* hold is drawing state the ADT has no reason to define — the
contribution index a history layout sorts on, the branch a generated archive stamped, and a
display label. Those sit **beside** the library's claim (`core/claims.ts`), never inside a copy
of it, and a test scans the tree to keep the deleted duplication from growing back.

## Server connections

Configured in the client, several at a time, switchable — the auth kinds mirror the
server's adapters and `openapi.yaml`'s security schemes: no-auth, `X-API-Key`,
`Authorization: Bearer` (JWT), and `Authorization: Macaroon`. Secrets stay in memory
for the session unless you tick *remember*, which writes them to `localStorage` where
any script on the page can read them.

A connection is read through the client generated from `openapi.yaml`
(`src/core/data/openapi.gen.ts`), so every route, path and response type a read uses is
the contract's and this repo hand-writes none of them. Auth stays the client's: which
kinds an instance accepts follows from how it was configured, so the generated client is
handed the headers the kind calls for rather than asked to negotiate one.

## Branches, and where a query is answered

The header's picker lists what the archive holds — the whole archive and each named
branch, each with the head its closure is rooted at — read through the data-source port,
so it works against a server (`GET /branches`) and against mock data alike. `$universe`
is absent: a browsable scope needs a head, and it has none, which is also why RankeQL
refuses to read there without an explicit one. No branch name is built in; `main` is a
fact about someone else's archive, so every name comes from the listing.

**The engine answers what a branch contains, not this client.** Selecting a scope runs a
query for identities only — `select.branch` names the scope, `output.detail: id` asks for
the ids — and the view then draws the intersection with what is cached. Switching scopes
therefore costs a query for ids and a `Set.has` per claim, never a re-read of a claim body.

An earlier attempt walked the closure here instead, which is a second query engine in the
layer least able to optimise one. It also cost **398 ms at 100k claims and 1.7 s at 300k**
for the widest scope. Both are reasons; the boundary is the better one.

The archive scope needs the branch-table head, which `GET /archive/info` reports — the only
route that does. Against an instance too old to answer it, or a subject without `R $archive`,
the picker offers the branches alone rather than a scope with no head.

A scope's answer can name claims this session never read — the archive advanced, or the
load was capped — so the count is reported next to the picker rather than the overlap being
drawn silently.

**A claim's provenance is a scope too.** Its closure has a root — the claim — so it is
browsable by the same rule, read within the branch the claim was reached through, since
`select.branch` wants a scope name and a claim id is a head. *show claim provenance* in the
selection pane opens one in its own graph view, with the canvas, layouts, camera and lens
every other view uses; one view per claim, and asking again brings it forward. The branch
picker leaves such a view alone — its scope is the claim it is named for — while still
setting the branch for the next read.

Membership is keyed by head rather than name, a head being what identifies a closure: two
claims' provenance read within one branch would otherwise share an answer.

The claims a closure names but the session lacks are read after the view is drawn. A
reference whose target has not arrived is held rather than discarded and drawn once it does,
because a discarded reference makes the claim that stated it look like an initial claim — a
claim with no references, which is the one thing this view exists to assert. Height settles
which it is: an initial claim carries 0.

## Open: stretching x without stretching y

The timeline already spreads the visible strata across the whole height, so a uniform zoom
wastes half of what it does — stretching y achieves nothing and only shrinks the bands when
zooming out. What is wanted is x alone.

Sigma has one uniform `ratio` and no per-axis zoom, so this means rewriting x and leaving y
where it is. What decides the design is that **the cost is in the nodes loaded, not the nodes
visible**: a stretch step rewrites x for every node and then pays an O(N) Sigma refresh,
whether ten are on screen or ten thousand. So the gate is the loaded count. Measured refresh
cost: **733 ms at 50k**, against a layout pass of 17 ms at 2k and ~100 ms at 100k — the
refresh dominates by an order of magnitude.

### The lens, which makes the cost go away

Rather than gating the stretch by a threshold, change the size of the problem: while zoomed
in, draw only the claims in the window. Two modes for two uses — seeing the whole archive, or
navigating it with a lens.

Sigma is handed a small graph in lens mode, so every stretch step is O(window) rather than
O(union), and rewriting x per zoom step becomes affordable.

Nothing is rebuilt on the way back: the union lives at module scope and the lens only reads
it. What a naive switch would cost is Sigma's own work — uploading N nodes into its buffers
and re-indexing them — which is avoidable by giving each graph its own Sigma instance and
switching which canvas is shown. The union's buffers stay valid while lensing, so neither
direction pays an upload. The price is two WebGL contexts, the union's GPU memory resident
throughout (as it already was), and camera state kept in step so the switch does not jump.

The bar is frictionless, and two things are what meet it: the lens is built and uploaded
*before* the canvases swap, so no frame is ever shown empty; and both instances are handed
the same camera state at the moment of the swap, so the picture does not move. Neither is
expensive — the lens is small — but getting the order wrong is what a reader would see.

Two things to settle first:

- **It bends the one-graph invariant, honestly.** The union stays authoritative; the lens is a
  derived, disposable copy for rendering, discarded on exit. Ids are content addresses, so
  selection and hover survive the switch without mapping.
- **Edges leaving the window are counted, not followed.** Bringing the far end along as a
  stub was the first design, and measuring settled it: provenance reaches back arbitrarily far
  in time, so a 4% window of 100k claims pulled in **8,333 stubs against 3,994 real claims**.
  A lens two-thirds composed of placeholders is not a lens. Instead each claim carries how many
  of its references leave the view, so the signal sits on the claim that has them.

Measured, with a viewport showing 4% of the axis:

| union | lens | cut once | stretch step: lens | stretch step: union |
|---|---|---|---|---|
| 20,000 claims | 801 (4.0%) | 54 ms | **2.8 ms** | 91 ms |
| 100,000 claims | 3,994 (4.0%) | 351 ms | **16.9 ms** | 591 ms |

So a stretch step costs about a thirtieth of what it would over the union, which is what makes
x-only zooming affordable at all. The cut is paid once per window rather than per step, and
only when a pan leaves the margin the window was widened by.

A measured node-count threshold with uniform zoom above it remains the fallback if the lens
proves more than is wanted.

# Performance

## What was measured, and what was not

| Leg | Measured | Where |
|---|---|---|
| Generate mock claims, build the graph, heap, wire size | ✅ | this container, headless |
| Height, contribution shape, all five layouts | ✅ | this container, headless |
| Sigma first paint, camera frame times, full redraw | ✅ | **a human's machine** — see below |
| VRAM | ❌ | not instrumented — needs `chrome://gpu` or a trace, by hand |

The headless numbers come from `make bench` in a container with no GPU and no
browser: `chrome-headless-shell` is missing 17 system libraries here (`libnspr4`,
`libnss3`, `libX11`, `libgbm`, …) and there is no root to install them, so
`make bench-render` **skips loudly and exits 0** — the convention the repo's adapter
tests use for podman-backed counterparts. A software rasteriser would have measured
SwiftShader rather than a GPU, so nothing was faked in its place.

**The container is capped at 2 GiB**, which is worth knowing because it briefly bound: at
ranke-ts 0.2.1 a claim cost 8 KB of heap, so 100k of them were 770 MiB, the
granularity sweep was OOM-killed at its third archive, and the 300k run could not finish at all.
0.3.0 brought that to 1.8 KB — 171 MiB at 100k, 509 MiB at 300k — and both runs fit again, so
the numbers below are measured at the shape they always were. `results/graph-bench-300k.json`
is back, and it earns its place: it is where the heap wall shows up (see Open questions).

**The frame numbers below were instead produced by a person running the page**, on
an integrated AMD Radeon (Renoir) through ANGLE / OpenGL ES 3.2 — a laptop-class
GPU, which is the realistic target. Their CPU-side figures also confirmed that the
headless numbers transfer: at 50k they measured build 723 ms and history layout 20 ms,
against 2 274 / 91 ms for 100k in this container at the time — linear, as expected, since
it is the same V8 doing the same work. (Their generate figure, 135 ms at 50k, is no longer
comparable to anything: the generator has since moved onto the ADT library's builder, which
is ~25× the work. Build and layout are unaffected — same topology, same graphology.)

**The render code was written without ever executing it**, and that showed. Two bugs
came out of it: Sigma v3 renders its first frame *synchronously inside the
constructor* (found by reading Sigma's source — awaiting an `afterRender` listener
attached afterwards waits forever), and a claim's ADT type was being stored in the
node attribute `type`, which Sigma reads as the name of the WebGL program to render
with — that one killed the first real run outright. Anything here still unexercised
should be assumed broken until a person has clicked it.

## The mock archive

Built **contribution by contribution**, because that is what gives a Ranke-Graph
its shape. Each contribution adds content claims, then a `contribution/head`
consolidating the previous head plus what is new, then a `contribution/branches`
revision carrying a `contribution/diff` to the table before it. Those two chains
are the commit history, and they are what makes the graph tall.

Content within a contribution is ordinary Ranke: `source/*` roots, `derivation/*`
citing recent inputs (locality → clusters), `entity/*` distilled from derivations,
`relation/*` joining entities with `relation_direction`, everything carrying a
`contribution/contributor` edge. Claims only reference *earlier* claims, so the DAG
is acyclic and `created_at` monotonic, as the ADT requires.

**The claims are real claims.** They are built by the ADT library's `claim_builder`, which
means the ids are the content addresses of the canonical encoding — `id = H(S(v))`, 56 chars of
multibase base32, identity-signed since no key is held here (§5.7) — the edge rules are
enforced as they would be on a server, and the type a generated claim has is the type a read
returns. Deterministic: same `--seed`, same archive.

That costs **~0.12 ms per claim** against ~0.01 ms for the object literals it replaced — 8 400
claims a second, so 10k in a little over a second and 100k in twelve.

It cost 2.5× that until ranke-ts 0.3.0, and how it came down is the useful part.
Taking a four-edge claim apart at 0.2.1 showed 243 µs, of which the cryptography was 8: each
record was encoded to compute its id, encoded again inside `encodeNode` to hash the node,
encoded a *third* time by `encodeClaim` for the stored bytes, and then parsed back by
`decodeClaim` into the record the builder had all along. 0.3.0 keeps the edge bytes it already
made (`encodeNodeWithEdges`), wraps the node bytes it already hashed (`encodeClaimFromNode`),
and builds the `Claim` from the record instead of decoding its own output
(`claimFromRecord`) — none of which changes a byte of what an id commits to.

Content is the one thing a generator cannot invent: it declares an external content address
and a size, so the sizes are realistic, but there are no bytes behind them — reading a
generated claim's content says so rather than returning something made up.

**Claims per contribution is swept, not assumed.** It is a usage pattern nobody can
know in advance, and it sets the archive's height — so the bench varies it and
reports cost as a curve. 2 % of contributions are bulk ingests 25× the usual size,
which is why the achieved mean sits above the nominal figure.

## Results

`node v24.18.0`, AMD Ryzen 7 PRO 5850U, 16 cores, 3 GiB heap ceiling under a 2 GiB container
cap. Single runs; ForceAtlas2 varies ±20 % between them, so it is quoted to two significant
figures — and, as the granularity table shows, it varies far more than that with what the heap
already holds. Raw: [`results/graph-bench.json`](results/graph-bench.json).

### Getting the data in

| claims | edges | generate | JSON of the claims | JSON.parse | build → graphology | graph on heap | claims on heap |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 000 | 3 787 | 0.15 s | 1.1 MiB | 5 ms | 14 ms | 1 MiB | 2 MiB |
| 10 000 | 37 709 | 1.3 s | 11.4 MiB | 52 ms | 156 ms | 13 MiB | 17 MiB |
| 100 000 | 377 121 | 8.8 s | 114.3 MiB | 712 ms | 2 128 ms | 123 MiB | 171 MiB |

**~1.2 KiB per node** in graphology, and **~1.8 KiB per claim** as objects. Building the graph
costs ~3× parsing a payload of the same claims, so the pre-paint bill is graphology's rather
than the wire's. Inspector metadata on every node (`full` vs `lean` attributes) adds ~6 % of
heap and nothing measurable in time — the two build columns land within the noise of each
other, so display attributes are not the problem.

The JSON column is the *decoded* claims stringified, which bounds a response from above rather
than being one: a wire record carries `type` once, not split, and no epoch milliseconds.

#### What a claim costs on the heap, and what it cost before 0.3.0

A decoded claim with four edges measures **1 813 B**, against **1 296 B** for the same data in
the old four-field `MockClaim` — so **~1.4×** buys a split type, both timestamp forms, a
generation number, a content declaration and a `fields` record per node and per edge. That is
the price of holding the model rather than a projection of it, and it is fair.

It was **8 070 B** at 0.2.1, and finding out why is worth recording, because the shape was never
the problem: the same data JSON round-tripped came to 1 728 B, and the shape hand-built came to
1 816 — within 3 B of what 0.3.0 now returns. The missing ~6 KB was V8 **ConsStrings**.
`base32Encode` built an id with `out += char` in a loop, so every id stayed a rope of ~57 nodes
at 32 B each; a heap snapshot over 5 000 claims counted **1.16 M concatenated strings, 35 of
56 MiB** — 232 ropes per claim, one per edge reference, per content hash, per id. Flattening
them in place recovered 74 % and changed no value, which is what made the diagnosis certain.
0.3.0 builds the digits into an array and joins them; flattening now recovers 5 B/claim, i.e.
nothing.

Two things the episode is worth remembering for. A rope is invisible to every structural
measure — `JSON.stringify` of the object graph, a field-by-field size model, a diff of the
shape — and shows up only in a heap snapshot or in what flattening gives back. And a heap
figure that big means an allocation pattern, not a data model: 30× the wire size of a record
should have been read as a bug from the start rather than as the cost of a claim.

### Shape: the two candidate axes

The archive is tall, but not in the way the DAG suggests. Provenance depth and
contribution order disagree, and the disagreement decides the layout:

| claims | contributions | height (depth) | widest depth layer | rows (history) | widest row | mean row |
|---:|---:|---:|---:|---:|---:|---:|
| 1 000 | 55 | 61 | 186 | 55 | 246 | 18.2 |
| 10 000 | 709 | 715 | 1 763 | 709 | 286 | 14.1 |
| 100 000 | 6 814 | 6 820 | **17 710** | 6 814 | **301** | 14.7 |

**Provenance depth is a bad axis.** Height tracks the contribution count, as
expected — but only the head/branch-table spine climbs it. A source claim citing
just its contributor sits at depth 1 however old the archive is, and a derivation
citing recent sources at depth 2–3, so 17 710 claims share one depth layer at 100k.
`y = depth` draws a tall thin spine beside a flat mat of everything else.

**Contribution order is the axis.** Same graph: 6 814 rows averaging 14.7 claims,
widest 301 — a 23:1 ribbon, one row per contribution, which is also what a reader
means by "when". It needs one number per claim (which contribution added it), and
that is why `contribution` is in the lean attribute profile: it is an axis, not
metadata. It is also the one number a *read* cannot supply — an archive expresses that
order as the head chain and no output field reports it — so it is drawing state the
explorer carries beside the claim rather than a field of one.

### Laying it out

| claims | by history | by depth | random | circular | circlepack | ForceAtlas2 (Barnes-Hut) | 200 iterations |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 000 | 1 ms | 2 ms | 1 ms | <1 ms | 11 ms | ~7 ms/iter | 1.4 s |
| 10 000 | 7 ms | 6 ms | 2 ms | 2 ms | 99 ms | ~87 ms/iter | 17 s |
| 100 000 | **108 ms** | 129 ms | 20 ms | 15 ms | 3 930 ms | ~2 000 ms/iter | 6.8 min |

- **The history axis is ~3 800× cheaper than a settled force layout at 100k**
  (108 ms against 406 s) and it is deterministic — same archive, same picture, every
  time, which a force layout never gives you.
- **ForceAtlas2 is the wall.** ~13× per 10× nodes to 10k, ~23× on the next decade. At
  100k one iteration misses a 60 fps frame budget by ~120×. A worker (`make dev` →
  *ForceAtlas2 (worker)*) moves the cost off the main thread without making it finish.
- **Barnes-Hut is not optional**: at 10k, off costs ~710 ms/iter against ~87.
- **Seeding barely matters**: from circlepack rather than random, 100k measured
  ~1 500 ms/iter against ~2 000 — same order, no rescue.
- **circlepack** groups by class attractively but costs 3.9 s at 100k. Fine to ~10k.
- **depth pass**: computing every claim's depth is one sweep, 316 ms at 100k — an
  O(V+E) property, which is why the layered layout is nearly free.

These land on the pre-library figures within the run-to-run noise, in both directions: build at
100k is 2 128 ms against 2 274 before, circlepack 3 930 against 3 916, ForceAtlas2 at 10k
87 ms/iter against ~130 — and at 100k 2 000 against ~1 500. A shared host with 24 of 43 GiB in
use by other work measured **28 % between two runs of the same build**, which is the size of
every gap above. Holding the decoded claims does not explain them either: with the archive
released before the layout, 50k claims measured 3 650 ms for three FA2 iterations against
3 375 ms with it held. The graph is the same graph, and nothing about it got dearer.

### Granularity: the parameter nobody can know

Same 100 000 claims, varying claims per contribution:

| claims/contribution | contributions | bookkeeping share | height | widest depth layer | rows | widest row | edges | FA2 |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| ~3 | 28 760 | **58 %** | 28 762 | 8 712 | 28 760 | 39 | 312 399 | ~1 630 ms/iter |
| ~10 | 6 814 | 14 % | 6 820 | 17 710 | 6 814 | 301 | 377 121 | ~1 630 ms/iter |
| ~30 | 2 530 | 5 % | 2 539 | 19 466 | 2 530 | 995 | 389 702 | ~1 710 ms/iter |
| ~100 | 638 | 1 % | 651 | 20 242 | 638 | 3 515 | 395 253 | ~1 690 ms/iter |
| ~1000 | 44 | 0 % | 65 | 20 486 | 44 | 35 805 | 396 664 | ~1 800 ms/iter |

Three things fall out, and the first is not about rendering at all:

1. **Fine-grained contributions make the archive mostly bookkeeping.** At ~3 claims
   per contribution, **58 % of all claims are heads and branch tables** — the archive
   is more history than content. At ~30 it is 5 %, at ~100 it is 1 %. That is a cost
   the server pays too (storage, closure walks, sequencer revisions), not just a
   picture problem.
2. **Layout cost is indifferent to granularity** (1 630–1 800 ms/iter across a 300× range).
   Edge count drives ForceAtlas2, and the edge count barely moves — 312k to 397k, because the
   head chain it loses is the head chain the content keeps citing; height does not drive it at
   all.
3. **The history axis degrades gracefully into uselessness.** At ~1000 claims per
   contribution there are 44 rows of ~2 270 — no longer a ribbon, and a renderer
   would need to lay out *within* a row. Below ~100 claims per contribution the
   axis works well; above that it needs a second dimension.

### Frame times on real hardware

Measured by a person driving the page, not by this container. Machine: integrated AMD
Radeon (Renoir) via ANGLE / OpenGL ES 3.2, Linux. Layout `history`, edges drawn during
motion (`hideEdgesOnMove` off), scripted camera path via **Run camera bench**.

| claims | edges | labels | edges while moving | first paint | camera | frame p50 | frame p95 | full redraw |
|---:|---:|---|---|---:|---:|---:|---:|---:|
| 50 000 | 188 114 | off | drawn | 1 005 ms | **6.8 fps** | 125 ms | 376 ms | 733 ms |
| 50 000 | 188 114 | on | drawn | 985 ms | — | — | — | — |
| 50 000 | 188 114 | off | **hidden** | 602 ms | **59.6 fps** | 16.7 ms | 16.9 ms | 560 ms |
| 50 000 | 188 114 | **on** | **hidden** | 781 ms | **58.1 fps** | 16.7 ms | 20.4 ms | 663 ms |
| 10 000 | 37 549 | on | drawn | — | ~20 fps | — | — | — |

The 10k figure is eyeballed from the live HUD rather than the scripted path, so treat
it as an order of magnitude. The first-paint and full-redraw differences between rows
one and two are machine warmth, not the mitigation — `hideEdgesOnMove` only affects
frames drawn *during* camera motion.

**Hiding edges while moving is the whole difference: 125 ms → 16.7 ms per frame, an
18× improvement that lands exactly on the 60 fps vsync cap.** p95 of 16.9 ms says the
renderer is idle-waiting for the display, so the frame budget is not merely met — it
has room to spare.

That room is the useful part, because hiding all edges **looks worse**: the picture
loses its structure exactly while you are moving through it. Taking the two rows as
two points, edges cost about (125 − 16.7) ms ÷ 188 114 ≈ **0.6 µs per edge drawn**.
Spending half a frame — 8 ms — on edges would then buy roughly **14 000 of them**,
about 7 % of the graph, held during motion instead of none. That is an extrapolation
from two measurements, not a measurement, and the obvious next experiment.

Three further things follow, and the first corrects a guess of mine:

1. **Labels are not the bottleneck; edges are — and labels are close to free.** I
   predicted the opposite, on the theory that Sigma draws labels as 2D canvas text
   every frame while nodes and edges go through WebGL. The controlled pair (rows three
   and four, identical but for labels) says otherwise: **58.1 fps with labels against
   59.6 without, the same 16.7 ms p50**, and only the p95 tail moves (20.4 against
   16.9 ms). First paint likewise barely notices them (985 against 1 005 ms).

   The likely reason is that Sigma's label culling is doing its job: labels only
   render above `labelRenderedSizeThreshold` (8 here) and the label grid caps their
   density, so at 50k zoomed out only a handful are ever drawn. That is a hypothesis
   about *why*, and it predicts labels get expensive when zoomed far enough in for
   many nodes to clear the threshold — which nobody has measured.
2. **First paint is ~1 s at half the target scale**, and a **full redraw is 733 ms** —
   so any data update that re-indexes the graph is a visible stall, independent of
   panning.
3. **The binary choice is avoidable.** Sigma offers all-edges or no-edges during
   motion; the first is 6.8 fps, the second loses the structure a reader navigates by.
   The frame budget says a middle exists — a few thousand edges held during motion,
   chosen by degree, zoom relevance, selection neighbourhood, or sampling.

   Better still, **gate edges on zoom level**: zoomed far out, 188k edges overlap into
   near-solid colour and carry no information, so hiding them there costs the reader
   nothing and buys the whole 18×. That also suggests the per-edge model above is too
   simple — a flooded view is fill-rate bound, so cost tracks the *pixels* edges cover,
   which makes long edges (the contributor star, spanning the entire picture)
   disproportionately expensive and worth dropping first.

   Note the implementation asymmetry: `hideEdgesOnMove` is an internal per-frame skip
   and free to toggle, whereas an `edgeReducer` marking edges `hidden` filters during
   indexation, so a zoom-threshold crossing would pay the full-refresh cost measured
   above (560–733 ms) unless a per-frame skip can be reached instead.

### The contributor star

Every claim carries a `contribution/contributor` edge, so a contributor's degree is
the number of claims it signed. With 4 contributors over 100k claims the top node
reaches **degree 25 120** — a quarter of the archive on one point — and it stays
~25k across the whole granularity sweep:

| claims | edges (all) | max degree | edges without contributor edges | max degree |
|---:|---:|---:|---:|---:|
| 1 000 | 3 775 | 268 | 2 779 | 198 |
| 10 000 | 37 549 | 2 549 | 27 551 | 297 |
| 100 000 | 376 634 | 25 120 | 275 636 | 494 |

Dropping them at render time removes **27 % of all edges** and cuts max degree ~50×.
It buys some layout time (~1 100 against ~1 500 ms/iter at 100k) but the real reason
is legibility: a star over every claim in the archive tells a reader nothing while
dragging every cluster towards one point. A rendering decision only — the edge stays
in the data, where verification needs it.

### Client footprint

The whole stack — React, graphology, Sigma, the layouts, the ADT library, the generated REST
client, the generator — bundles to **494 KB (142 KB gzipped)**, or one 494 KB self-contained
`explorer.html`. The two libraries this client stopped hand-writing cost ~38 KB of that, for
the claim model, the codec, the sequence readers, the query type and every route.

## What this implies for an explorer

1. **Don't force-lay-out an archive.** Use contribution order: 91 ms at 100k,
   deterministic, and semantically right. Keep ForceAtlas2 for a drill-down of a
   few thousand claims, where it costs 13–130 ms/iter and settles in seconds.
2. **Budget edges, not layout.** Layout is solved at 21 ms; drawing 188k edges every
   frame is not, at 6.8 fps. Hide edges while the camera moves, decimate them by
   zoom level, or drop whole edge classes (starting with the contributor star, which
   is 27 % of them). This is where the next spike's effort belongs.
4. **Provenance depth is not the history axis**, even though it correlates with it.
   Measure before believing otherwise — I did believe otherwise.
5. **Budget ~2.5 s and ~235 MiB for a 100k read** before layout or paint: 0.3 s
   parse, 2.3 s build, 97 MiB claims, 138 MiB graph. Drop the claim objects once the
   graph is built. Add ~1 s of first paint on top, measured at 50k.
6. **Never draw `contribution/contributor` edges by default** — 27 % of edges,
   actively harmful to the picture, and now also implicated in the frame cost.
7. **Contribution granularity is a first-class design parameter**, and it belongs to
   whoever writes to the archive, not to the renderer. At the fine end the archive is
   mostly bookkeeping; at the coarse end the history axis stops separating anything.

## Open questions

- **Partial edges while moving.** Hiding all of them hits 60 fps; hiding none gives
  6.8 fps; ~0.6 µs per edge suggests ~14 000 could stay visible inside the budget.
  Nobody has measured a partial draw, and the interesting variants differ in what they
  keep: highest-degree edges, edges incident to the selection, edges within the rows
  currently on screen, or a flat random sample. Sigma has no setting for this — it
  needs an `edgeReducer` that hides by predicate, and re-measuring.
- **Dropping the contributor edges** (27 % of them) as a cheaper first cut, which
  should be worth ~1.6 of the 6.8 fps case on the per-edge figure — probably not
  enough alone, which is itself worth confirming.
- Frame times at 100k, and the same runs on a discrete GPU — though if the cost is
  edge upload and fill rate, an integrated GPU is the honest target anyway.
- Whether 300k survives a browser tab. It builds: 33 s to generate, 509 MiB of claims,
  a 378 MiB graph, 7.8 s to build it, and the history layout still lands in **324 ms**.
  Then ForceAtlas2 takes **145 s per iteration** — against 2.0 s at 100k, a **72× jump for
  3× the nodes**, which is not a complexity curve but a heap wall: 509 + 378 MiB of live
  objects under a 1.9 GiB ceiling leaves every iteration collecting rather than computing.
  It is the same cliff an earlier run hit on a 1 120 MiB ceiling (132 s/iteration, then
  `Ineffective mark-compacts near heap limit`), and it is the clearest measurement in this
  file of one thing: **peak heap is a first-order performance factor**, and a browser tab's
  ceiling sits right there. Which also says what to try — stop holding the page. Merging each
  claim into the graph as it arrives and dropping it takes 509 MiB out of the read, and the
  streaming reader already makes it possible. The deterministic layouts, meanwhile, do not
  care: 324 ms at 300k.
- **Labels when zoomed in.** They cost nothing at 50k zoomed out because Sigma culls
  almost all of them; the same camera path zoomed into a dense contribution row, where
  many nodes clear `labelRenderedSizeThreshold`, is the case that would actually price
  label drawing.
- Whether rows should be contributions or time buckets once contributions get large.
- Whether progressive rendering (draw at 10k, keep streaming) beats waiting.

## Files

| Path | What |
|---|---|
| `Makefile` | The entry point — `run`, `build`, `single`, `bench`, `test`, `check` |
| `dist/explorer.html` | Generated, committed: open it with no npm and no server |
| `src/main.tsx` | Mounts React, which renders the shell |
| `src/core/connections.ts` | The instances this client can talk to, and their credentials |
| `src/core/data/source.ts` | The data-source port and its two backends: a connection, and the generator |
| `src/core/data/openapi.gen.ts` | Generated, committed: the REST client, from the root `openapi.yaml` |
| `src/core/claims.ts` | The library's claim, plus the drawing state it has no reason to define |
| `src/core/mock/model.ts` | The subtypes a generator invents, and the shape of a generated archive |
| `src/core/mock/generate.ts` | Contribution-by-contribution archive generator |
| `src/core/graph/` | Claims → graphology: the union, the lens, scope membership, depth and degree statistics |
| `src/core/layout/` | The layouts, and the timescale the history layout stretches |
| `src/core/store.ts`, `session.ts` | View state, and the actions a read or a scope change runs |
| `src/render/renderer.ts` | The one Sigma instance and the reducers a view is expressed through |
| `src/ui/` | The shell, its panes, and the header |
| `src/bench/graph-bench.ts` | The headless bench (every number above) |
| `src/bench/browser-bench.ts` | Playwright driver for frame timings; skips cleanly |
| `src/**/*.test.ts` | The headless core tests — `make test` |
| `results/*.json` | Measured output, committed on purpose |
