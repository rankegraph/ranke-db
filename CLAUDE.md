# ranke-db — Agent Instructions

## What this repo is

ranke-db is the **server** for the Ranke graph — a slim, **hexagonal** wrapper around
the `ranke-go` library. It launches **exactly one stack** from a config file and serves
it. It is **not** a graph implementation.

**The boundary, always:** graph model + verification = **`ranke-go`**; server, the
adapter ports, config, and access policy = **`ranke-db`**. Never reimplement graph logic
here — compose the library.

## Library: ranke-go (a versioned GitHub module)

Depend on ranke-go as a normal module: `require github.com/rankegraph/ranke-go vX.Y.Z`,
bumped via semver. **ranke-go and ranke-ts mirror each other**: `go.mod`'s ranke-go and
`frontend/package.json`'s `@rankegraph/ranke` implement one wire format, and a
disagreement leaves the explorer computing ids no server agrees with — a failure with no
build error. The two version lines are independent: where they happen to
align it is coincidence, so neither the numbers nor a reading of them side by side says
anything about whether the two agree. ranke-ts asserts the agreement itself, testing
byte for byte against ranke-go; `make -C frontend test` is where that reaches this repo,
and it is the check after bumping either.
**NEVER** wire it as a sibling path, `go.work use`, or `replace` — and
never edit a sibling `ranke-go` checkout. It provides the data model (claims, Universe,
BranchTableHead, Archive), verification, and the **Storage** + **Sequencer** adapters.

## Read the spec FIRST — obligatory before ANY design/API work

The source of truth is the **`ranke-graph`** paper repo (Typst `.typ`). **Read it before
designing — do not reconstruct the model from memory or from this file.** Fetch the raw
sources directly (WebFetch), or `make docs-papers` to pull copies into `docs/papers/`
(gitignored; `make docs` pulls them and then builds this repo's own handbook):

- **normative specification — the rules an implementation FOLLOWS** (cite rule ids like `R-CEIL`):
  `https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/main/spec/ranke-spec.typ`
- **ranke-db — architecture & data model** (read this for the server):
  `https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/main/02-ranke-db/ranke-db.typ`
- **ranke-graph — foundational model & philosophy**:
  `https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/main/01-ranke-graph/ranke-graph.typ`
- **glossary — series-wide terminology**:
  `https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/main/glossary/ranke-glossary.typ`

The papers govern meaning; the spec must never contradict them (Paper 01 wins for ADT rules,
Paper 02 for RankeDB rules). `make docs-papers` pulls **all** of them — papers, spec,
glossary, shared.

**Never diverge from the papers** — to change a concept, get consensus and update the paper
first. The paper is the user's to write; do not edit it.

**Anchors the spec pins (so you don't misdesign — but still read it):**
- A **claim** is **signed CBOR**, content-addressed by `id = Sign(H(S(claim)))`. The
  **contributor** (an application-held key) signs the claim → its id; the server's separate
  **signing identity** attests the *merge*. So a client knows a claim's id before sending it.
- A **branch table** is itself a claim (`contribution/branches`, edges `contribution/branch`
  → each named graph head); **B_h** = the id of its latest revision. `(Universe, B_h)` = a
  Ranke-Archive. The **Sequencer** advances B_h (keeps its history for rollback). Branches
  are handles, not resources; their heads are moving targets.
- Storage: **Blob store** (`get/put/has`, content-addressed) → typed **Universe** →
  composed via **Stack** (eager/lazy layers) and **Partition** (sharding). "Layers" = a Stack.
- A read is a **filtered query**: the closure of a head, narrowed by a *conjunction of
  filters*, capped by a result limit (conjunctive monotonicity). Out-of-scope refs come back
  as **hash-only stubs** so the subgraph still Merkle-verifies. (Cypher is NOT in the spec.)
- Access is **scope-based**: a base posture (`allow-all`/`deny-all`) + an ordered list of
  wildcard exceptions over field/edge names & types, bound to the authenticated account.
- **Verification** has three depths: completeness (`has` sweep) / record-correctness
  (recanonicalise + recheck id-chain & signatures) / full-content (re-hash blobs).
- Writes carry a **transaction window** `[t_open, t_commit]`; the server merges atomically
  and records the witnessed window in the signed merge (the *hard* timestamp; `created_at`
  is soft/forgeable).
- **Config = the launch artifact**: one JSON doc, `env()`/`vault()` references, age
  encryption; **validate** is offline/secret-free, **resolve** happens only at launch.
- **6 adapter ports**: storage, sequencer, signer, secret-store (vault), authenticator,
  endpoints (transport+authenticator pairs: REST+JWT, MCP+macaroon, …).
- "**One process serves one configuration**" — the process boundary is the design.

## Architecture: the hexagon (6 adapter ports)

One stack, assembled from one config file, no runtime reconfiguration. Admin cycle =
edit config → run → observe → stop → edit → repeat.

- **Driven** (core drives them): **Storage** (𝒰) + **Sequencer** (B_h) — from ranke-go
  (InMemory/FileSystem/S3/Postgres/neo4j and InMemory/FileSystem/Postgres); **Signer**
  (a `crypto.Signer` for contributing claims) + **Vault** (secret KV) — here.
- **Driving** (they drive core): **Auth** (request→subject) + **Endpoints** (REST/HTTP,
  MCP/HTTP) — here.
- Postgres/neo4j/S3/Azure/OpenBao are optional **external plugins**; the default mem/fs
  path needs none.

**Config = the composition root** (`config/`): JSON, with optional whole-file age
encryption (decrypted by core before parse). Any string field may be `env(KEY)` or
`vault(KEY)` — resolved at `run` (env first, then build Vault from the env-only vault
section, then resolve `vault(...)`). `verify` parses + schema-checks only (offline; never
touches env/vault). Accounts + grants are a **static config section**.

**Access** (`access/`): a struct request answered by a checker — `Action ∈ {Read ⊂ Write
⊂ Admin}` (read = query incl. Cypher-read; write = contribute; admin = privileged). No
runtime grant mutation.

## OpenAPI-first

`openapi/openapi.yaml` is the **single source of truth** for the REST API. `make generate`
produces every artifact from it **into `openapi/`** (alongside the spec):
- **Go** server interface + models → `openapi/openapi.gen.go` (oapi-codegen, strict net/http)
- **TS/JS** client → `openapi/openapi.gen.ts` (swagger-typescript-api), copied to
  `frontend/src/core/data/openapi.gen.ts` and committed there — the explorer reads an instance
  through it and `frontend/` builds without this target
- **HTML** reference → `openapi/openapi.html` (Redocly) · **Markdown** reference → `openapi/openapi.md` (widdershins)

The two references are also symlinked under `docs/openapi/` (`openapi.html`, `openapi.md`) so
they're browsable there without a second copy.

Never hand-edit `*.gen.*` or the generated client/references — change the spec and regenerate.

## Tooling & shell conventions

- **Trust the project's own tooling.** The `make` targets are built and tuned for this
  workflow; their output is concise and meaningful. Run them **directly**.
- **Avoid compound commands.** No `&&` / `;` chaining, and do **not** pipe target output
  through `tail`/`grep`/`head` or redirect it to "manage" it. One command per invocation —
  it reads clearly, fails clearly, and avoids spurious permission prompts. If you feel the
  urge to chain or filter, that's a signal the target is missing or noisy — **fix the
  target**, don't paper over it.
- **`make`** (default) = `generate` then `verify` — the full build (regenerate from the
  spec, then check it).
- **`make verify`** is the green-gate: it first runs `generate`, then builds, vets,
  gofmt-checks, tests, lints (`brokkr lint`, falling back to `sindri lint`), and builds the
  handbook (`docs-pdf`) — so a vocabulary change upstream breaks the gate here too. Run it
  after changes instead of hand-rolled `go build`/`go test` chains. (`generate` shells out to
  `npx`, so the first run fetches the doc/client tools.)
- **`make generate`** regenerates everything from the OpenAPI spec into `openapi/` (Go server,
  TS client, HTML + Markdown references) and refreshes the `docs/openapi/` symlinks.
- **`make check-tools`** reports any missing tool (go + node + typst) with install hints, holds
  node to the floor the generation tools need and typst to the pinned series (`TYPST_SERIES`, the
  minor being what changes a layout) — `verify` needs typst now, since `docs-pdf` is behind the
  gate. A release installs `TYPST_VERSION` exactly.
- **`make upgrade`** bumps the dependencies in one shot — every dep (`go get -u -t ./...`), the
  tool deps, then **ranke-go last** so a pin sticks — tidies, and runs `verify`, so a breaking
  upstream change surfaces here. The **`go` directive is not swept along**: it asks first and
  puts back what `go get -u` raises on its own (`GO_VERSION=keep` / `=1.26.5` to skip the
  prompt). Pin the library with `RANKE_GO_VERSION=vX.Y.Z`; revert with
  `git checkout go.mod go.sum`.
- **`make docs`** = `docs-papers` (pulls the upstream papers, spec and glossary into
  `docs/papers/`, and places `docs/vocabulary.typ`/`docs/handbook.typ` for `docs/`'s own
  chapters) then `docs-pdf` (builds `dist/docs.pdf` via typst, pinned to the version
  ranke-graph's own release.yml uses). `make docs-current` refreshes the placed files only if
  ranke-graph moved (one `git ls-remote`) — the freshness check `verify` runs before
  `docs-pdf`. The format a chapter follows is ranke-graph's **`docs-spec/ranke-docs-spec.typ`**,
  with `docs-spec/examples/docs-tree/` as the tree to copy — `docs-spec/` is outside the fetched
  set, so read both at
  `https://raw.githubusercontent.com/rankegraph/ranke-graph/refs/heads/main/docs-spec/`. A chapter
  imports `vocabulary.typ` and nothing else, and reaches for the constructs rather than raw
  markup: `item` for a flag/key/field, `listing` for a code block, `note`/`warning`, `example`,
  `gls`/`glspl` for glossary terms (`G-GLS`). The construct list is `shared/constructs.typ`,
  which the fetch does bring in — **`make docs-check`** reads the groups from it and holds
  `docs/` to `G-CHAPTER`, `G-IMPORT`, `G-CONSTRUCTS` and `G-NOLAYOUT`, since compiling proves
  none of them. It runs ahead of `docs-pdf`, so `verify` covers it. **`make docs-bundle`** packs what
  this repo AUTHORED — root, chapters, `assets/`, and `openapi/openapi.gen.yaml` — into
  `dist/ranke-db-docs.tar.gz`, which the release carries beside `docs.pdf`, so a consumer takes
  one asset instead of cloning. Nothing fetched goes in: the templates, the papers and
  `rql.schema.json` are ranke-graph's to publish, and a second copy here would be a second
  source of truth.

## Changelog

`CHANGELOG.md` records what each release changed for someone who depends on
this repository. A change earns an entry when it alters what the repo requires,
provides, or removes; rewording does not. Write the entry under `## Unreleased`
in the same change, and summarise: one entry per change that matters to a
reader, not a log of every edit it took to get there.

Group the entries under `### Added`, `### Changed`, `### Fixed` and
`### Removed`, adding a heading only when something goes under it. Name what
moved: a config key, a flag, a route, a command, a dependency and its version.
Say what a reader must now do differently, and let a removal name its
replacement — "`sequencer.history` … replace it with `seed`" tells a reader
what to edit, where "dropped the history section" leaves them to work it out.
Where a fix corrects behaviour someone may have relied on, say what the old
behaviour was, since that is how a reader recognises the bug they hit.

`make release <bump>` stamps that section with the version it cuts, leaves a
fresh `## Unreleased` behind, and commits it on the branch being released, so
a version heading is never written by hand. A release whose `## Unreleased`
section is empty is refused, since it would record nothing. The stamping lives
in ranke-graph's shared `release-cycle.sh`, which this repo caches under
`bin/`, and where `CHANGELOG.md` is missing the first release writes it.

A stamped section keeps its group headings, so read the file and find
`## Unreleased` before writing rather than anchoring an edit on `### Fixed` or
`### Removed`. A release cut since you last looked leaves those headings inside
a shipped version, where an entry claims a change that release never carried
and leaves `## Unreleased` empty — which the next release then refuses.

## Layout

Classical single-module Go repo at the repo root (module `github.com/rankegraph/ranke-db`).

- `openapi/` — the API spec (source of truth) **and** its generated artifacts (Go server `openapi.gen.go`, TS client `openapi.gen.ts`, `openapi.html`, `openapi.md`; the two references symlinked under `docs/openapi/`)
- `cmd/ranke-db/` — the server binary (`run <config>`, `verify <config>`; later `tui`/config edit)
- `cmd/generator/` — a **client** that seeds a running instance over `POST /contribute`:
  it derives its own contributor identity, signs its own claims, and sends them as a
  contribution stream. Shapes: `example` (4 claims), `release` (the release process drawn in
  `docs/use-case-release-process.png` — four attested signing identities, two packages meeting
  at one release, logs worth reading) and `chain` (many contributions). Seeding is never a
  server feature — a contributor is an application-held key. `make dev SEED=example|release|chain`
  launches and seeds; `make seed SEED_URL=…` seeds a running instance
- `internal/core/` (+ `internal/core/access/`) · `config/` · `adapters/<port>/` — the hexagon
  (each `adapters/<port>/<port>.go` holds the port contract + its `New` factory; the
  backends sit in subpackages)
- `docs/`, `openspec/` — docs & specs

- `frontend/` — the **Ranke Explorer**: a pure browser client (Vite + React + TypeScript,
  graphology + Sigma v3 + zustand). Static bundle, no application server, no proxy, no
  database of its own; it talks straight to a ranke-db REST endpoint, holds several
  instances at once, and works with none at all against mock data. Its own
  `package.json` and its own `Makefile`, which the root one reaches at one point only:
  `make dev` builds `dist/explorer.html` first, since `-tags explorer` embeds it at compile
  time and a committed bundle is whatever was last released. Otherwise it stands alone —
  `make -C frontend dev` for the dev server, `make -C frontend` to build the distributable.

  Layering is strict and one-way: `core/` is **headless** (store, graph, layouts, mock
  data, connections — no React, no DOM, no Sigma), `render/` owns the Sigma instance at
  module scope, and `ui/` is pure interface (React components that read state and
  dispatch actions, holding no graph data). Node and edge data must never enter React
  state, context or props.

Status: mid-refactor on `refactor/hexagonal` — the pre-hexagonal server (tenants,
multi-stack, and the schemaf-era explorer) is purged; the spec + `make generate`/`verify`
pipeline is in place; `core`/adapters/`cmd` are being built. The explorer is rebuilt from
scratch under `frontend/` (epic `td-976f37`) and reads a live instance through the client
generated from the merged REST contract (`rest-api`, from `add-rest-api`) — a committed copy
of `openapi.gen.ts` under `frontend/src/core/data/`, so `frontend/` stays buildable on its own.
