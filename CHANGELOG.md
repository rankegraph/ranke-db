# Changelog

What each release changed for someone depending on this repository.

## Unreleased

### Changed

- `ranke-client branch create` contributes one claim: the creator's contributor claim,
  carrying the pubkey and dated when it was added. That brings the branch into being and
  is what every later claim on it resolves through (`V-SIG`). Each branch gets its own,
  so no branch references another and the command needs no **R** on `$archive` — a
  `$`-target no tenant holds. Within a branch the claim is reused rather than added again.
- `client.NewContributor(keypair)` replaces `client.RegisterContributor`, returning
  `ranke.Contributor` rather than a wrapper of its own.

### Removed

- `contribution/branch_created`. A branch's creation is recorded by the Sequencer, in the
  branch-table revision that adds the entry; a client's claim restating it added a second
  record of the same event. Nothing replaces it — read the branch table
  (`GET /branches`, or `ranke-client branch list`).
- `client.Client.ResolveContributor`, `client.Contributor`, `client.RegisterContributor`,
  and the sentinels `ErrContributorLapsed`, `ErrContributorAmbiguous`,
  `ErrNoSuchContributor` and `ErrContributorUnresolved`, all from v1.25.0. Resolving a key
  against the archive was what made a branch reference another branch's contributor. Build
  the claim with `client.NewContributor`; to see what the archive holds, read
  `client.Client.Contributors` or run `ranke-client contributor list`.
- `ranke-client branch create --contributor` and `--register-identity`, both from
  v1.25.0. Each existed to steer the resolve.

## v1.25.0 — 2026-09-10

### Added

- `client.Client.ResolveContributor(ctx, keypair, at, pick)` answers the contributor a
  signing key signs as: the `contribution/contributor` claim the archive already holds
  for that pubkey, or a freshly minted one where the key has never contributed. It
  refuses rather than guessing where the answer is not single — `ErrContributorLapsed`
  where every claim over the key is outside its validity window at `at` (`R-C4KEY`),
  `ErrContributorAmbiguous` where several are valid at once, `ErrNoSuchContributor`
  where `pick` names a claim the key does not carry, and `ErrContributorUnresolved`
  where the read was forbidden. Needs the **R** right on `$archive`.
- `client.RegisterContributor(keypair)` mints the claim for a first-time contributor,
  epoch-dated so one key yields one id however often a caller runs.
- `client.Client.Contributors(ctx)` and `client.Client.Expiries(ctx)` read the archive's
  `contribution/contributor` and `contribution/expiry` claims;
  `client.ContributorsFor(claims, pubkey)` picks a key's own out of them.
- `client.ContributorWindow`, with `client.ContributorWindowOf` and
  `client.ContributorWindows`, reads a contributor key's validity as `R-DEXPIRY` fixes
  it: the bounds the claim states, shortened by the earliest `contribution/expiry` edge
  naming it. `Admits(at)` answers the question `R-C4KEY` asks of every claim it signs.
- `ranke-client branch list` reports every branch the table holds with the head it
  resolves to, and the branch-table head above them, that being the id an `$archive`
  query or grant is held against. `--deep` adds each head's height and when the branch
  last moved, one request per branch. Needs the **R** right on `$branches`.
- `ranke-client contributor list` reports each contributor the archive holds — id,
  pubkey, when it was added, its key window and whether that window admits now — and
  names any pubkey carried by more than one claim, which is how a forked identity
  becomes visible. `--signing-key` narrows it to one key's own.
- `ranke-client branch create --contributor <id>` names which contributor claim to sign
  as, for a key that carries several. `--register-identity` registers the key without
  reading the archive first, for an account holding no **R** on `$archive`.

### Changed

- `ranke-client branch create` now also wants the **R** right on `$archive`, to resolve
  the signing key against the contributors already registered there. An account without
  it is refused, naming both the grant to add and `--register-identity`.

### Fixed

- `ranke-client branch create` contributed a second `contribution/contributor` claim for
  its signing key on every run, so a key the archive had already registered — the
  founder key, registered at founding — ended up with two contributor claims over one
  pubkey, leaving every claim signed under it ambiguous as to which contributor it
  resolves through. The key is now resolved against the archive and the registered claim
  referenced across the branch boundary, a registration travelling only for a key the
  archive does not hold. The two claims could never have converged by agreeing on a
  date: a founding contributor is attested by the server, carrying a
  `contribution/contributor` edge and a height above 0, where a client-minted one is an
  initial claim — different references, so different ids whatever they are dated.

## v1.24.0 — 2026-09-10

### Added

- `client`, the official Go client for a running instance — `client.New(baseURL,
  opts...)` over every one of the contract's 21 operations. It wraps
  `openapi/client`, so a spec change breaks the build rather than drifting; a bare
  `host:port` is read as `http`, and a request the caller supplied no `http.Client`
  for is bounded by `client.DefaultTimeout`. `cmd/generator` and `cmd/ranke-client`
  are its first two consumers and carry no HTTP of their own.
- Credentials are one per client, refused at construction: `client.WithToken`,
  `client.WithAPIKey`, `client.WithMacaroon`, or none for a NoAuth endpoint.
  `WithMacaroon` is separate because the endpoint routes `Authorization: Bearer` to
  the JWT authenticator, where a macaroon 401s with nothing naming the cause.
- A refused request is a `*client.Error` carrying the contract's `code`, matching
  the sentinel for its category through `errors.Is` — `ErrUnauthenticated`,
  `ErrForbidden`, `ErrNotFound`, `ErrConflict`, `ErrBusy`, `ErrInvalid`,
  `ErrUnimplemented`.
- `client.DecodeRecord` reads one record of a result sequence in either framing —
  RFC 7464 json-seq or RFC 8742 cbor-seq — into ranke-go's `QueryResult`, the
  trailing execution report included. `Client.QueryRaw` hands back the split records
  untouched, `Client.Query` the decoded ones, and `Client.QueryClaims` pins the
  stored-envelope output whose bytes re-hash to the id (`R-QCANON`).
- `ranke-client --macaroon` and `generator --macaroon` present a base64 macaroon,
  the one credential carrying caveats.
- `rest_http.Server.Handler()` returns the routed endpoint without a listener under
  it, for a caller driving the routes directly.

### Fixed

- A query setting `execution.report` now ends its result sequence with the report,
  as `rest-api` requires. The report element reached the record writer, which has no
  payload field for it, and the write failed silently — so every reported run came
  back with its results and no report at all.

## v1.23.0 — 2026-09-10

### Added

- Every binary carries a `version` subcommand — `ranke-db version`, `ranke-client
  version`, `generator version` — printing the build it is. `make build` compiles and
  stamps all three, where it built two and stamped one.

### Changed

- A release carries `ranke-client` beside `ranke-db`, and both are stamped with the tag
  they were cut from rather than leaving the version to the toolchain's VCS stamping.
- The release matrix drops `darwin-amd64`, leaving `linux-amd64`, `linux-arm64` and
  `darwin-arm64`.

## v1.22.0 — 2026-09-10

### Added

- `ranke-client`, a second binary for the client half. It holds a contributor key and
  addresses a server already running, where `ranke-db` operates an instance from its
  config and makes no request. Keeping them apart keeps two unlike keys apart: a
  contributor key signs claims into their ids, the server's identity attests the merge.
  It links none of the storage or vault drivers.
- `ranke-client whoami` reports the account a credential resolves to, its grants and any
  caveats.
- `ranke-client branch create <name> --signing-key <spec>` creates a branch by
  contributing the claim that records it, together with the creator's contributor claim.
  That pairing is the point: a branch whose closure reaches no contributor admits no
  writer, and the record's reference is what pulls one in. A branch that already exists
  is reported and left alone, so it can run before every deployment.
- `--signing-key` takes a path, `file:PATH`, `env:NAME`, `stdin` or `prompt`, through
  ranke-go's `keysource`. A key file is refused unless `0600`, a key given as the flag's
  own value is refused and reported as compromised — a command line reaches the process
  table, shell history and any CI log — and an unrecognised scheme is named rather than
  read as a filename.
- A Go client generated from `openapi.yaml` into `openapi/client/`, so a client tracks
  the contract rather than restating it.
- `make dev` builds `frontend/dist/explorer.html` first. `-tags explorer` embeds it at
  compile time, so a committed bundle meant the dev loop served whatever was last
  released.
- The explorer names its own build under the wordmark, stamped from `git describe` the
  way the binary is, and reports each connection's server version on the Server tab.

### Changed

- ranke-go v0.31.0, whose `keysource` package and `ParseKeypair` replace a hand-written
  copy of both here.

### Fixed

- The Server tab probes its connections when it opens. The probe existed but ran only
  from a button inside an expanded row, so the tab listed names and no health at all.

## v1.21.0 — 2026-09-10

### Changed

- ranke-go v0.30.0. Founding now binds the archive's first contributor to a branch,
  which is what makes that contributor reachable: `V-ARCHIVEHEIGHT` allows k₀ one
  reference, so it cannot name the contributor itself. An archive founded without a
  branch held that claim referenced by nothing, and nobody could ever write to it.
- `sequencer.founder` becomes the `sequencer.found` section: `found.pubkey` is the
  first contributor's PEM public key as before, and `found.branch` names the branch
  the archive begins on. `found.branch` is required whenever an archive is founded,
  since which branch it starts on is permanent. `ranke-db found` reports it alongside
  the head and bookmark.
- A branch name is `[a-z0-9_]` with no leading underscore, which `ranke.ValidateBranchName`
  now enforces. Grant globs follow the same charset, so `CR foo-*` becomes `CR foo_*`
  and a grant can no longer name branches that cannot exist. Existing configurations
  using hyphens in branch names or globs must be rewritten.

### Added

- `GET /system/whoami` reports the caller back to itself: the account its credential
  resolved to, that account's grants, and any caveats attenuating them. It needs no
  grant, so an account holding nothing still gets an answer, and a bad credential is
  still `401`. Grants and caveats come back separately rather than intersected, since
  a request needs both to allow it — which is the only way to see what survived a
  macaroon attenuation.

## v1.20.1 — 2026-09-09

### Fixed

- The release workflow reads the typst pin again. `make print-typst-version` ran the
  paper fetch as a prerequisite, so its progress lines reached stdout alongside the
  version and `$GITHUB_OUTPUT` received several lines where it takes one — failing the
  docs job, and with it the release, on `Invalid format`.

## v1.20.0 — 2026-09-09

### Added

- `ranke-db found <config> <pubkeyfile>` founds the archive a config points at, under
  the PEM public key of its first contributor, and exits. An archive that already
  exists is reported as such and left alone, so provisioning may call it ahead of
  every launch. Only the public half ever reaches the server.
- `sequencer.seed` and `sequencer.bookmark` name the bookmark list an instance
  advances — a seed for a list from index 0, a bookmark id for one reopened through a
  surviving entry. Exactly one is required and the two are mutually exclusive. A seed
  is any non-empty string: it is a name, not a secret, its entropy only keeping
  uncoordinated lists apart.
- `sequencer.founder`, the PEM public key a launch founds an archive under. Absent it,
  a launch that finds no archive refuses to serve and names what is missing rather
  than answering every request from an archive that is not there.
- A launch that founds an archive logs the first contributor, the head and a bookmark
  id. Keep the bookmark: with the seed lost, it is the only way back to the archive.
- A storage tree that can hold no bookmark store is refused when a sequencer is
  configured over it, naming the configured layers and the composition rule that
  applies. A rebuildable projection holds none, the head being lost with the reindex.
- `GET /` answers `ranke-db <version>` as plain text. It sits outside the API contract,
  beside `/explorer`; every other unrouted path stays a `404`.
- `GET /health` carries `version`, the build answering the request. A release names
  itself exactly; a build from a checkout names its revision, marked when the tree
  carried uncommitted changes. The field is required, so a client may rely on it.
- The explorer reports a connection's server version beside its health in the
  connections pane.
- `make build` stamps the version through `-ldflags`, taking it from `git describe`
  (override with `VERSION=`). A binary built without it falls back to the module
  version or the revision Go recorded, so it always names itself.

### Changed

- ranke-go v0.29.0 and `@rankegraph/ranke` 0.29.0. The head timeline is now a list of
  signed bookmarks in 𝒰_hist, which belongs to the Universe, so it inherits the
  layering, replication and backup of the storage beneath it.
- A restart reopens the archive its bookmark list records. Both earlier releases
  founded unconditionally at construction, so a relaunch over persistent storage
  published a second and empty head above the real archive and left it unreachable,
  its claims intact but unreferenced.
- A malformed time bound in a query answers `400` rather than `500`. A comparison on a
  time field takes one spelling (`R-QTIMEOP`): a `V-TIME` timestamp on `created_at`,
  `delete_by`, `pubkey_valid_from` and `pubkey_expires_after`, an EDTF Level 1 value
  on `dated`.
- A request reaching a server whose archive is not founded answers `503`.

### Removed

- `sequencer.history`. The bookmark store is the Universe's own, so a detached
  𝒰_hist — a file head timeline over an in-memory universe — can no longer be
  expressed, and the section has nothing left to point at. Replace it with `seed`.
