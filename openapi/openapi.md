---
title: ranke-db API v0.2.0
language_tabs:
  - shell: Shell
  - http: HTTP
  - javascript: JavaScript
  - ruby: Ruby
  - python: Python
  - php: PHP
  - java: Java
  - go: Go
toc_footers: []
includes: []
search: true
highlight_theme: darkula
headingLevel: 2

---

<!-- Generator: Widdershins v4.0.1 -->

<h1 id="ranke-db-api">ranke-db API v0.2.0</h1>

> Scroll down for code samples, example requests and responses. Select a language for code samples from the tabs above or the mobile navigation menu.

REST/HTTP binding for a single **ranke-db** stack: read the verifiable graph
with a declarative query, and contribute signed claims to it.

A server hosts exactly **one stack**, launched from a config file — there is
no tenant or archive routing in the paths.

## Reads are queries

The full read surface is `POST /query`, carrying a **RankeQL** query — the
declarative `Query` type the normative spec fixes (§RankeQL) — as a JSON object:
`select` generates the result set, `where` filters it, `order` sorts it, `limit`
truncates it, and `output` shapes and encodes each surviving claim. This contract
*binds* that type to HTTP and defines none of it; the field names, values and
meanings are the spec's. Cypher/GQL is **never** a client route — it is an
internal execution engine the planner lowers a query to.

A **cacheable GET subset** covers the by-id reads without a query body. Every one
of them is a **scope** followed by what is read within it, so the three scopes the
access model reserves read alike and differ only in which scope they name:

  - `GET /branches` — the branch table's branches, by name and head
  - `GET /branches/{branch}/head`
  - `GET /branches/{branch}/claims/{id}` and its `/content` form
  - `GET /archive/claims/{id}` and its `/content` form — the `$archive` scope
  - `GET /universe/claims/{id}` and its `/content` form — the privileged
    `$universe` scope

`GET /branches` needs no branch name, so a client discovers what it may address.
Content is addressed by the **claim** that holds it (not a raw hash), so whether the
bytes are inline or a separate blob is hidden and the read stays scoped as its route
is scoped. Content also rides **inline** in query results via `output.content`; the
content route fetches the bytes for a single claim — including whatever a capped read
truncated or dropped, since a claim keeps its `content_hash` either way.

No path segment carries a `$`. The reserved names live in grants (`R $universe`) and
in a RankeQL body (`select.branch`), and a route names the same scope as a plain
segment — which also means these paths survive being typed into a shell.

## Scopes and closures

`select.branch` is the mandatory **scope**: a branch name confines the query to
that branch, `$archive` to the whole Ranke-Archive, and `$universe` applies no
confinement — the privileged by-head-id read, which therefore **requires**
`select.head`. Under every other scope `head` is optional and narrows the read to
that claim's closure, so it can only narrow, never widen past the grant. A closure
is immutable, so pinning `head` gives a paged read a snapshot that cannot shift
while the archive advances.

## Encodings and verifiability

A query returns a **streaming sequence** of results, one item each, in the query's
order. `output.encoding` fixes how each item is serialised and the response media
type frames the sequence:

  - `json` → `application/json-seq` (RFC 7464) — JSON records, content
    base64-encoded when inlined. A **convenience projection**: easy to read and
    debug, but **not** independently verifiable.
  - `cbor` → `application/cbor-seq` (RFC 8742) — binary.

Verifiability is a property of the *shaping*, not of the framing alone:
`detail: claims` + `form: original` + `encoding: cbor` + `content: {max: 0}`
reproduces the canonical serialization a claim's id is computed over, and is the only
combination directly re-hashable and signature-checkable against that id (`R-QCANON`).
Every other shaping is a rendering for convenience.

**The content axis is part of that combination, not a detail of it.** A query inlines
no content unless `output.content` asks (`R-QCONTENT`), so a read shaped for
verification states `content: {max: 0}` — a cap of zero meaning *in full*, as a zero
bound does everywhere else in a query. Omit it and the claims arrive without their
content, which re-hashes to something other than the id. `content_size` is served
either way, so a client always sees that content exists and how long it is.

A by-id claim GET returns the claim as its signed CBOR (`application/cbor`) whole, so
it needs none of this; a content GET streams the blob as raw bytes
(`application/octet-stream`). In query results content is instead carried inline
(base64 under `json`, byte strings under `cbor`), bounded by `output.content`.

## Credentials and authorization

The contract **binds no per-operation rights** and defines **no** read/write/admin
ladder — authorization is entirely core-access (system accounts with CRUDA rights
over branch globs), decided on the subject the credential resolves to. Every route
accepts the same set of credentials; the shape of a route never varies by right.

What the contract *does* enumerate is how a credential is **presented**, because
the endpoint routes deterministically on the presented scheme to the auth adapter
of that type — no parsing a token to guess its kind:

  - `Authorization: Bearer <jwt>` → the **JWT** adapter
  - `X-API-Key: <key>` → the **API-key** adapter
  - `Authorization: Macaroon <token>` → the **macaroon** adapter
  - **no** credential header → the **noauth** adapter

Which of these adapters is actually mounted is the stack's config; a presentation
with no configured adapter is `401`. (Only if two mechanisms were forced onto one
scheme would the endpoint fall back to trying each — the schemes above are
distinct, so routing stays deterministic.) A `403` body never discloses the
subject id.

## Browsers

A browser refuses a cross-origin read unless the server admits the origin, so an
instance an explorer is meant to reach declares which origins may: the endpoint's
`allowedOrigins` (a comma-separated list, `*` for any). Declaring none — the default —
leaves the API unreachable from any page but its own origin, which is right for a server
nobody browses.

Where an origin is admitted, a preflight is answered with the methods and the credential
headers above, and `ETag` is **exposed**, without which the conditional reads this
contract describes cannot be made from a script. No origin is granted credentialed
access: a credential rides in a header here, never in a cookie.

## Closure and the 404 rule

Every branch read is bounded by that branch's closure, and every archive read by
the current head's closure. A claim or content that exists in the Universe but lies
**outside** the closure the route names returns `404` — indistinguishable from one
that does not exist. Reads under no closure at all are privileged: they are the
`/universe/…` collection, gated by **R** on `$universe`.

Base URLs:

* <a href="/">/</a>

# Authentication

- HTTP Authentication, scheme: bearer `Authorization: Bearer <jwt>` → the JWT auth adapter.

* API Key (apikey)
    - Parameter Name: **X-API-Key**, in: header. `X-API-Key: <key>` → the API-key auth adapter.

- HTTP Authentication, scheme: macaroon `Authorization: Macaroon <token>` → the macaroon auth adapter.

<h1 id="ranke-db-api-read">read</h1>

Read the graph — the query surface and the cacheable by-id GETs.

## Read the graph with a declarative query

<a id="opIdquery"></a>

`POST /query`

Runs a RankeQL query (§RankeQL) and streams the result set, one item per
result, in the serialization chosen by `output.encoding`. Results carry the
natural `(created_at, id)` order unless `order` sorts them, with its keys
applied in priority order and that natural order breaking any remaining ties;
to page, carry the last result's order key into the next request's `where`
(pin `select.head` so the closure cannot shift between pages).

When `execution.report` names a verbosity, the **final item** in the sequence
is a `QueryReport` (see the schema) — typed distinctly from result items and
always last — carrying the execution log, timing, and whether a limit truncated
the read. `execution.layer` pins which storage layer runs it.

The response media type mirrors `output.encoding`: `application/json-seq` or
`application/cbor-seq`. A branch named in `select` that does not exist is
`404`; claims outside the scope's closure simply do not appear. `$universe`
is the privileged unconfined read and requires `select.head`.

> Body parameter

```json
{
  "select": {
    "branch": "$universe",
    "head": "string",
    "claim": "string",
    "path": [
      {
        "edges": [
          "string"
        ],
        "dir": "provenance",
        "min": 0,
        "max": 0,
        "nodes": [
          "string"
        ]
      }
    ]
  },
  "where": {
    "and": [
      {
        "and": []
      }
    ]
  },
  "output": {
    "shape": "single",
    "detail": "id",
    "form": "original",
    "content": {
      "max": 0,
      "overflow": "cutoff"
    },
    "encoding": "json"
  },
  "order": [
    {
      "field": "string",
      "compare": "numeric",
      "dir": "asc"
    }
  ],
  "limit": {
    "results": 0,
    "time": "string"
  },
  "execution": {
    "layer": "string",
    "report": "info"
  }
}
```

<h3 id="read-the-graph-with-a-declarative-query-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[Query](#schemaquery)|true|none|

> Example responses

> 200 Response

> 400 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="read-the-graph-with-a-declarative-query-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The result sequence. One item per result, plus an optional trailing
`QueryReport` when `execution.report` is set. The content type is the one
matching `output.encoding`.|string|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|The request was malformed.|[Error](#schemaerror)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## List the branch table's branches

<a id="opIdlistBranches"></a>

`GET /branches`

Returns every branch the branch table holds, each with its name and current
head id. Reachable **without knowing any branch name**, so a client discovers
what it may address before using the routes that take one; an archive with no
branches yet answers with an empty list.

Requires the **R** right on the reserved `$branches` target, which is what
core-access grants as "enumerates the table".

Cacheable **with revalidation** (weak `ETag`, `Cache-Control: no-cache`): every
head in the list moves as its branch is contributed to. The listing is answered
from **one archive snapshot**, so the heads are consistent with each other.

> Example responses

> 200 Response

```json
{
  "branches": [
    {
      "name": "string",
      "head": "string"
    }
  ]
}
```

<h3 id="list-the-branch-table's-branches-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The branches, each by name and current head.|[BranchList](#schemabranchlist)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Weak validator for the branch table's current state.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Current head id of a branch

<a id="opIdgetBranchHead"></a>

`GET /branches/{branch}/head`

Returns the branch's current head id — a moving target (it advances on every
contribution). Requires the **R** right on that branch. Cacheable **with
revalidation** (weak `ETag`, `Cache-Control: no-cache`): a conditional request
is cheap when the head has not moved. To inspect the head claim itself, fetch it
via `GET /branches/{branch}/claims/{id}`.

<h3 id="current-head-id-of-a-branch-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|branch|path|string|true|The branch name (`$` is reserved and illegal in ordinary names).|

> Example responses

> 200 Response

```json
{
  "head": "string"
}
```

<h3 id="current-head-id-of-a-branch-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The current head id.|[BranchHead](#schemabranchhead)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Weak validator for the current head.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## What is known about a branch

<a id="opIdgetBranchInfo"></a>

`GET /branches/{branch}/info`

Reports a branch beyond its head id: the head's **height** — the generation number
of the branch's newest claim, so the depth of what it points at — and **when it last
moved**, which is the head claim's `created_at`.

Everything here comes from the head claim, so it costs one claim read. A claim count
is deliberately absent: counting a branch's claims is a walk of its closure, which is
a query (`POST /query`) and not a field.

Requires the **R** right on the branch. Cacheable **with revalidation** — every
field moves when the branch does.

<h3 id="what-is-known-about-a-branch-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|branch|path|string|true|The branch name (`$` is reserved and illegal in ordinary names).|

> Example responses

> 200 Response

```json
{
  "name": "string",
  "head": "string",
  "height": 0,
  "updatedAt": "2019-08-24T14:15:22Z"
}
```

<h3 id="what-is-known-about-a-branch-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|What is known about the branch.|[BranchInfo](#schemabranchinfo)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Weak validator for the branch's current state.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## What is known about the archive

<a id="opIdgetArchiveInfo"></a>

`GET /archive/info`

Reports the Ranke-Archive as a whole: the **branch-table head** — the id every
archive-scoped read is rooted at — with its height and when it last moved, and how
many branches the table holds.

This is the only route that reports the branch-table head, which is what a client
needs to name the `$archive` scope in a query or a grant.

Reads the `$archive` scope and requires the **R** right on `$archive`. Cacheable
**with revalidation**: the head advances on every contribution.

> Example responses

> 200 Response

```json
{
  "head": "string",
  "height": 0,
  "updatedAt": "2019-08-24T14:15:22Z",
  "branches": 0
}
```

<h3 id="what-is-known-about-the-archive-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|What is known about the archive.|[ArchiveInfo](#schemaarchiveinfo)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Weak validator for the archive's current state.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch a claim within a branch's closure

<a id="opIdgetBranchClaim"></a>

`GET /branches/{branch}/claims/{id}`

Returns claim `{id}` as its signed CBOR bytes, **only if it lies in branch
`{branch}`'s closure**. A claim that is superseded, contradicted, or otherwise
outside the closure returns `404`, indistinguishable from one that does not
exist. Requires the **R** right on that branch.

`{branch}` names an **ordinary branch**. A reserved scope name supplied here
names a branch that does not exist and is answered as `404`; each scope has
exactly one route (`/archive/…`, `/universe/…`).

Immutably **cacheable** by id (strong `ETag`, `Cache-Control: public,
immutable`): the id content-addresses the bytes, so they never change.

<h3 id="fetch-a-claim-within-a-branch's-closure-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|branch|path|string|true|The branch name (`$` is reserved and illegal in ordinary names).|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-a-claim-within-a-branch's-closure-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The claim's signed CBOR bytes.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch the content of a claim within a branch's closure

<a id="opIdgetBranchClaimContent"></a>

`GET /branches/{branch}/claims/{id}/content`

Streams the content of claim `{id}` — **only if it lies in branch `{branch}`'s
closure** (same closure guarantee as the claim itself; out-of-closure or
unknown → `404`). Requires the **R** right on that branch. Content is addressed
by the claim that holds it, not by a raw hash: the server resolves whether the
bytes live inline in the claim or in a separate blob, so the client can't tell
and doesn't need to — and the read stays scoped as the route is scoped.
Immutably **cacheable** by the claim id.

<h3 id="fetch-the-content-of-a-claim-within-a-branch's-closure-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|branch|path|string|true|The branch name (`$` is reserved and illegal in ordinary names).|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-the-content-of-a-claim-within-a-branch's-closure-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The content bytes, streamed.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch a claim within the archive head's closure

<a id="opIdgetArchiveClaim"></a>

`GET /archive/claims/{id}`

Returns claim `{id}` as its signed CBOR bytes if it lies in the closure of the
**whole Ranke-Archive** — the current branch-table head — whichever branch holds
it. A client reaches a claim without naming the branch it is on; a claim outside
that closure returns `404`.

This collection reads the `$archive` scope: the same scope a RankeQL body names
as `select.branch: "$archive"`, and the same target a grant is written against.
It requires the **R** right on `$archive`.

Immutably cacheable by id.

<h3 id="fetch-a-claim-within-the-archive-head's-closure-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-a-claim-within-the-archive-head's-closure-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The claim's signed CBOR bytes.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch the content of a claim within the archive head's closure

<a id="opIdgetArchiveClaimContent"></a>

`GET /archive/claims/{id}/content`

Streams the content of claim `{id}` if it lies in the archive head's closure,
across every branch (same closure guarantee as the claim itself; outside it or
unknown → `404`). Reads the `$archive` scope and requires **R** on `$archive`.
Content is addressed by the claim; whether the bytes are inline or a separate
blob is hidden. Immutably cacheable by the claim id.

<h3 id="fetch-the-content-of-a-claim-within-the-archive-head's-closure-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-the-content-of-a-claim-within-the-archive-head's-closure-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The content bytes, streamed.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch a claim by id from the Universe (privileged)

<a id="opIdgetClaim"></a>

`GET /universe/claims/{id}`

Returns claim `{id}` as its signed CBOR bytes from the Universe under **no
closure at all** — no branch table, and no confinement to the current head.
This is what makes it privileged, and what it exists for: reaching an archive
from a Universe and a head id alone, as when restoring from a head kept outside
the server. A claim the Universe holds is returned even where
`GET /archive/claims/{id}` reports it not-found.

This collection reads the `$universe` scope: the same scope a RankeQL body names
as `select.branch: "$universe"`, and the same target a grant is written against.
It requires the **R** right on `$universe`, to which only **R** applies. An
ordinary glob confers it by no accident — a `$`-prefixed target needs an exact
grant, so `R *` reaches neither reserved scope.

Immutably cacheable by id.

<h3 id="fetch-a-claim-by-id-from-the-universe-(privileged)-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-a-claim-by-id-from-the-universe-(privileged)-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The claim's signed CBOR bytes.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Fetch the content of a claim by id from the Universe (privileged)

<a id="opIdgetClaimContent"></a>

`GET /universe/claims/{id}/content`

Streams the content of claim `{id}` from the Universe under no closure — the
privileged read, conferred only through **R** on `$universe`. Content is
addressed by the claim; whether the bytes are inline or a separate blob is
hidden. Immutably cacheable by the claim id.

<h3 id="fetch-the-content-of-a-claim-by-id-from-the-universe-(privileged)-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|id|path|string|true|The content-addressed claim id.|

> Example responses

> 200 Response

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="fetch-the-content-of-a-claim-by-id-from-the-universe-(privileged)-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The content bytes, streamed.|string|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|ETag|string||Strong validator — the claim id.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

<h1 id="ranke-db-api-write">write</h1>

Contribute signed claims.

## Contribute signed claims (atomic)

<a id="opIdcontribute"></a>

`POST /contribute`

Contributes one or more signed claims to a branch as a single **atomic**
merge — all are absorbed under one new branch-table head, or none are.
Content-addressed and therefore **idempotent**: re-contributing yields the same
ids with no duplicates.

The body is ranke's contribution stream: a **CBOR sequence** (RFC 8742) of
records, self-framing so a contribution of any size streams.

  - `[2, ["<branch>", …]]` — **first**, the branches this contribution touches
  - `[0, <id>, <claim bytes>, "<branch>"]` — a claim, as the canonical CBOR its
    contributor signed, and the branch it joins
  - `[1, <hash>, <content bytes>]` — externalized content, which lives in the
    Universe unbranched and so names no branch

The header comes first so the **C** right is settled on every declared branch
before any of the body is read; a claim naming an undeclared branch is refused, so
the declaration binds. One contribution may therefore advance **several branches**,
and an unauthorized one is answered without its payload being read.

Payloads are CBOR byte strings carried through untouched: a claim's id is a
signature over exactly those bytes, so the server stores what it was sent. The id
is explicit because it cannot be derived — `Sign(H(S(node)))` is made with the
contributor's key, and the canonical encoding carries only the node. Content is
checked against the hash addressing it.

Send every claim the closure needs that the archive may not hold yet — a
first-time contributor includes its own `contribution/contributor` claim, since
everything it signs references it.

The sequencer mints the `contribution/branches` branch-table claim recording the
merge (paper 02 §Sequencer). On success the new branch-table head id and the
contributed claim ids are returned.

> Body parameter

<h3 id="contribute-signed-claims-(atomic)-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|string(binary)|true|none|

> Example responses

> 201 Response

```json
{
  "head": "string",
  "ids": [
    "string"
  ]
}
```

<h3 id="contribute-signed-claims-(atomic)-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|201|[Created](https://tools.ietf.org/html/rfc7231#section-6.3.2)|The merge committed (or the claims were already present).|[ContributionResult](#schemacontributionresult)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|The request was malformed.|[Error](#schemaerror)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|
|409|[Conflict](https://tools.ietf.org/html/rfc7231#section-6.5.8)|The contribution conflicts with the branch's current head.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

<h1 id="ranke-db-api-system">system</h1>

Operate the stack — liveness, storage introspection, verification runs.

## Liveness and signer identity

<a id="opIdhealth"></a>

`GET /health`

Reports that the stack is up and the signing/contributor identity it
attests merges under. Whether this requires a privileged subject is a
core-access decision, not part of this contract.

> Example responses

> 200 Response

```json
{
  "status": "ok",
  "version": "v1.19.2",
  "signer": "did:key:z6Mk..."
}
```

<h3 id="liveness-and-signer-identity-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The stack is up.|[Health](#schemahealth)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## List storage layers

<a id="opIdlistStorageLayers"></a>

`GET /system/layers`

Lists the stack's storage layers (read-through tiers) by **name and type
only** — never connection details or secrets. Naming layers is what lets a
verification run target one directly (a read-through view can mask the loss
of an object on a deeper layer).

> Example responses

> 200 Response

```json
{
  "layers": [
    {
      "name": "string",
      "type": "string"
    }
  ]
}
```

<h3 id="list-storage-layers-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The storage layers, top (cache) to bottom (authoritative).|[StorageLayerList](#schemastoragelayerlist)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## List verification runs

<a id="opIdlistVerifications"></a>

`GET /system/verifications`

Verification runs across the whole stack, newest first. Each report is a
**point-in-time record**: a layer repaired externally shows clean in a later
run, so reports accumulate rather than overwrite — until explicitly removed
with `DELETE /system/verifications/{reportId}`.

The API lives under `/system/` because a verification run is a ranke-db
extension beyond the paper's surface rather than archive content: it is a
stack-wide operational resource, rooted at a closure named in the request body
and not in the path. (Branch reads no longer occupy the root, so nothing here
is avoiding a collision.)

> Example responses

> 200 Response

```json
{
  "reports": [
    {
      "id": "string",
      "config": {
        "closure": "string",
        "layer": "string",
        "depth": "completeness",
        "contentThreshold": 0
      },
      "head": "string",
      "status": "running",
      "startedAt": "2019-08-24T14:15:22Z",
      "completedAt": "2019-08-24T14:15:22Z",
      "claimsChecked": 0,
      "bytesRead": 0,
      "ok": true,
      "failures": [
        {
          "id": "string",
          "mode": "corrupt-bytes",
          "layer": "string",
          "detail": "string"
        }
      ]
    }
  ]
}
```

<h3 id="list-verification-runs-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The verification reports.|[VerificationReportList](#schemaverificationreportlist)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Start a verification run

<a id="opIdstartVerification"></a>

`POST /system/verifications`

Starts a run from a `VerificationConfig`: walk the closure rooted at
`closure` (a branch name — resolved to its current head and **pinned** for
the life of the run — or a head id directly), reading the named `layer`
directly, re-checking each claim to the configured depth. It returns
*findings*, not contents, so it never leaks across closures.

A run may take a **very long time** (hours to days over a large closure at
full-content depth), so it is always **asynchronous**: the call returns `202`
immediately with the running report and a `Location` header pointing at the
report resource. Poll it with `GET /system/verifications/{reportId}`, pacing
by the `Retry-After` hint; stop it with `DELETE`. Starting the same run twice
yields two independent point-in-time reports (runs are not deduplicated).

Verification is resource-heavy, so the stack caps the number of runs that may
execute **concurrently** (configured; default 1). When that cap is already
reached the call returns `429` — the server never stops a run to make room. To
proceed, either wait (per `Retry-After`) or `GET /system/verifications` to see
what is running and free a slot deliberately: `cancel` a run to stop it while
keeping its report, or `DELETE` it to remove it entirely.

> Body parameter

```json
{
  "closure": "string",
  "layer": "string",
  "depth": "completeness",
  "contentThreshold": 0
}
```

<h3 id="start-a-verification-run-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[VerificationConfig](#schemaverificationconfig)|true|none|

> Example responses

> 202 Response

```json
{
  "id": "string",
  "config": {
    "closure": "string",
    "layer": "string",
    "depth": "completeness",
    "contentThreshold": 0
  },
  "head": "string",
  "status": "running",
  "startedAt": "2019-08-24T14:15:22Z",
  "completedAt": "2019-08-24T14:15:22Z",
  "claimsChecked": 0,
  "bytesRead": 0,
  "ok": true,
  "failures": [
    {
      "id": "string",
      "mode": "corrupt-bytes",
      "layer": "string",
      "detail": "string"
    }
  ]
}
```

<h3 id="start-a-verification-run-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|202|[Accepted](https://tools.ietf.org/html/rfc7231#section-6.3.3)|The run started; the running report is returned.|[VerificationReport](#schemaverificationreport)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|The request was malformed.|[Error](#schemaerror)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|429|[Too Many Requests](https://tools.ietf.org/html/rfc6585#section-4)|The concurrent-run cap is reached. No run was started and none was
cancelled; retry after a running one finishes or is stopped.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|202|Location|string||The report resource to poll — `/system/verifications/{reportId}`.|
|202|Retry-After|integer||Suggested seconds to wait before the first poll.|
|429|Retry-After|integer||Suggested seconds to wait before retrying.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Show a verification run

<a id="opIdgetVerification"></a>

`GET /system/verifications/{reportId}`

The report for a run. Poll until `status` leaves `running`; while it is still
running the response carries a `Retry-After` hint and the progress counters
(`claimsChecked`, `bytesRead`) advance between polls.

<h3 id="show-a-verification-run-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|reportId|path|string|true|The verification report id.|

> Example responses

> 200 Response

```json
{
  "id": "string",
  "config": {
    "closure": "string",
    "layer": "string",
    "depth": "completeness",
    "contentThreshold": 0
  },
  "head": "string",
  "status": "running",
  "startedAt": "2019-08-24T14:15:22Z",
  "completedAt": "2019-08-24T14:15:22Z",
  "claimsChecked": 0,
  "bytesRead": 0,
  "ok": true,
  "failures": [
    {
      "id": "string",
      "mode": "corrupt-bytes",
      "layer": "string",
      "detail": "string"
    }
  ]
}
```

<h3 id="show-a-verification-run-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The verification report.|[VerificationReport](#schemaverificationreport)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

### Response Headers

|Status|Header|Type|Format|Description|
|---|---|---|---|---|
|200|Retry-After|integer||While `status` is `running`, suggested seconds before the next poll.|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Delete a verification run

<a id="opIddeleteVerification"></a>

`DELETE /system/verifications/{reportId}`

Really deletes the run and its report. If the run is still `running` it is
stopped first, then the record is removed — so this both aborts a run and
cleans up finished history in one step. Unlike the cancel action, nothing
survives: a subsequent `GET` is `404`. Either verb frees a concurrency slot;
cancel keeps the report, delete removes it.

<h3 id="delete-a-verification-run-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|reportId|path|string|true|The verification report id.|

> Example responses

> 401 Response

```json
{
  "code": "string",
  "error": "string"
}
```

<h3 id="delete-a-verification-run-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|204|[No Content](https://tools.ietf.org/html/rfc7231#section-6.3.5)|The run was stopped (if running) and its report deleted.|None|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

## Cancel a running verification run

<a id="opIdcancelVerification"></a>

`POST /system/verifications/{reportId}/cancel`

Stops a `running` run but **keeps** its report: the record stays in history
with `status` `stopped` and whatever partial findings it had gathered, and the
concurrency slot is freed. Use this to abort a run you want to keep a record
of; use `DELETE` to remove it entirely. Idempotent — cancelling a run that has
already finished or stopped returns its current report unchanged.

<h3 id="cancel-a-running-verification-run-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|reportId|path|string|true|The verification report id.|

> Example responses

> 200 Response

```json
{
  "id": "string",
  "config": {
    "closure": "string",
    "layer": "string",
    "depth": "completeness",
    "contentThreshold": 0
  },
  "head": "string",
  "status": "running",
  "startedAt": "2019-08-24T14:15:22Z",
  "completedAt": "2019-08-24T14:15:22Z",
  "claimsChecked": 0,
  "bytesRead": 0,
  "ok": true,
  "failures": [
    {
      "id": "string",
      "mode": "corrupt-bytes",
      "layer": "string",
      "detail": "string"
    }
  ]
}
```

<h3 id="cancel-a-running-verification-run-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The run's report after the cancel (status `stopped` if it was running).|[VerificationReport](#schemaverificationreport)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|404|[Not Found](https://tools.ietf.org/html/rfc7231#section-6.5.4)|The branch, claim, or content is unknown — or lies outside the named
branch's closure. The two are indistinguishable.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

<h1 id="ranke-db-api-dev">dev</h1>

Development-only capabilities, mounted only when the stack is launched with --dev.

## Advance the dev sequencer's clock

<a id="opIdadvanceDevClock"></a>

`POST /dev/clock`

Available **only** when the stack was launched with `--dev` against a `dev`
Sequencer: moves the clock the Sequencer mints `created_at` and branch-table
timestamps from forward to (at least) the given instant, so a client that
knows its own story's schedule — a fixture generator, say — can make the
archive's *recorded* history track its *narrated* one, one contribution at a
time, rather than every merge landing at the real wall-clock moment the
client happened to run.

The clock never moves backward: a request older than its current position
is accepted and answered with the position unchanged. Absent `--dev`, or
against a `concurrent` (production) Sequencer, the route is `501` — the
witnessed merge time stays real, which is the whole point of `R-C2DATE`.

> Body parameter

```json
{
  "time": "2019-08-24T14:15:22Z"
}
```

<h3 id="advance-the-dev-sequencer's-clock-parameters">Parameters</h3>

|Name|In|Type|Required|Description|
|---|---|---|---|---|
|body|body|[DevClockAdvance](#schemadevclockadvance)|true|none|

> Example responses

> 200 Response

```json
{
  "time": "2019-08-24T14:15:22Z"
}
```

<h3 id="advance-the-dev-sequencer's-clock-responses">Responses</h3>

|Status|Meaning|Description|Schema|
|---|---|---|---|
|200|[OK](https://tools.ietf.org/html/rfc7231#section-6.3.1)|The clock's new position.|[DevClock](#schemadevclock)|
|400|[Bad Request](https://tools.ietf.org/html/rfc7231#section-6.5.1)|The request was malformed.|[Error](#schemaerror)|
|401|[Unauthorized](https://tools.ietf.org/html/rfc7235#section-3.1)|Authentication is required or failed.|[Error](#schemaerror)|
|403|[Forbidden](https://tools.ietf.org/html/rfc7231#section-6.5.3)|Access was denied by core-access. The body carries no subject id or
onboarding hint.|[Error](#schemaerror)|
|501|[Not Implemented](https://tools.ietf.org/html/rfc7231#section-6.6.2)|An optional capability the request needs is not configured.|[Error](#schemaerror)|

<aside class="warning">
To perform this operation, you must be authenticated by means of one of the following methods:
None, jwt, apikey, macaroon
</aside>

# Schemas

<h2 id="tocS_Query">Query</h2>
<!-- backwards compatibility -->
<a id="schemaquery"></a>
<a id="schema_Query"></a>
<a id="tocSquery"></a>
<a id="tocsquery"></a>

```json
{
  "select": {
    "branch": "$universe",
    "head": "string",
    "claim": "string",
    "path": [
      {
        "edges": [
          "string"
        ],
        "dir": "provenance",
        "min": 0,
        "max": 0,
        "nodes": [
          "string"
        ]
      }
    ]
  },
  "where": {
    "and": [
      {
        "and": []
      }
    ]
  },
  "output": {
    "shape": "single",
    "detail": "id",
    "form": "original",
    "content": {
      "max": 0,
      "overflow": "cutoff"
    },
    "encoding": "json"
  },
  "order": [
    {
      "field": "string",
      "compare": "numeric",
      "dir": "asc"
    }
  ],
  "limit": {
    "results": 0,
    "time": "string"
  },
  "execution": {
    "layer": "string",
    "report": "info"
  }
}

```

A read, evaluated in a fixed logical order: select generates the result set, where filters it, order sorts it, limit truncates it, output shapes and encodes each surviving claim (R-QEVAL).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|select|[Select](#schemaselect)|true|none|A generator in four independent parts: branch is the scope, head the closure read, claim where the walk starts, path the traversal. Scope and start are independent because a walk runs both ways — a uses step reaches the claims that cite the current one, which lie above it, so the closure decides a reverse step's answer.|
|where|[Where](#schemawhere)|false|none|A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT).|
|output|[Output](#schemaoutput)|false|none|Shapes each result along orthogonal axes. detail: claims with form: original and encoding: cbor reproduces the canonical serialization S(v) a claim's id is computed over, and is the only output form directly verifiable against that id (R-QCANON).|
|order|[Order](#schemaorder)|false|none|Sort keys applied in priority order. Claims lacking a key's field sort last, and the archive's natural (created_at, id) order breaks any remaining ties, so the sort always resolves to a total order (R-QSORT).|
|limit|[Limit](#schemalimit)|false|none|Bounds the read. A read cut short by either bound is a complete answer to the query as bounded, not an error (R-QLIMIT).|
|execution|[Execution](#schemaexecution)|false|none|Where the query runs and how it reports on itself. These controls reach execution and diagnostics, and never the result set.|

<h2 id="tocS_QueryReport">QueryReport</h2>
<!-- backwards compatibility -->
<a id="schemaqueryreport"></a>
<a id="schema_QueryReport"></a>
<a id="tocSqueryreport"></a>
<a id="tocsqueryreport"></a>

```json
{
  "started_at": "2019-08-24T14:15:22Z",
  "elapsed_ns": 0,
  "results": 0,
  "truncated": true,
  "events": [
    {
      "at_ns": 0,
      "engine": "string",
      "op": "string",
      "level": "error",
      "duration_ns": 0,
      "detail": "string",
      "attrs": {}
    }
  ]
}

```

Diagnostic report emitted as the **final item** of a `POST /query` sequence
when `execution.report` names a verbosity. Typed distinctly from result items,
so a reader never mistakes it for data, and always last.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|started_at|string(date-time)|false|none|Wall clock at query start.|
|elapsed_ns|integer|false|none|Total execution time in nanoseconds. The unit is in the name because the<br>value is a bare integer, and it is nanoseconds because a `trace` report<br>exists to show steps a millisecond would round away.|
|results|integer|false|none|Number of result items emitted before this report.|
|truncated|boolean|false|none|True if `limit.results` or `limit.time` cut the read short.|
|events|[[QueryEvent](#schemaqueryevent)]|false|none|The ordered, multi-engine execution log, at the requested verbosity.|

<h2 id="tocS_QueryEvent">QueryEvent</h2>
<!-- backwards compatibility -->
<a id="schemaqueryevent"></a>
<a id="schema_QueryEvent"></a>
<a id="tocSqueryevent"></a>
<a id="tocsqueryevent"></a>

```json
{
  "at_ns": 0,
  "engine": "string",
  "op": "string",
  "level": "error",
  "duration_ns": 0,
  "detail": "string",
  "attrs": {}
}

```

One entry in a query's execution log — a stage, a routing decision, or a
translation.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|at_ns|integer|false|none|Offset from `started_at` in nanoseconds.|
|engine|string|false|none|Who emitted it (e.g. native, cypher, stack, partition).|
|op|string|false|none|What it did (e.g. load-root, step, filter, sort, route, translate-cypher).|
|level|string|false|none|The entry's own level; a report carries everything at or above the<br>verbosity `execution.report` asked for.|
|duration_ns|integer|false|none|Elapsed time for a timed step, in nanoseconds; 0 for a point event.|
|detail|string|false|none|Human-readable message, or the translated query text (e.g. the Cypher).|
|attrs|object|false|none|Structured extras — layer or shard name, depth, edge and result counts, …|

#### Enumerated Values

|Property|Value|
|---|---|
|level|error|
|level|warn|
|level|info|
|level|debug|
|level|trace|

<h2 id="tocS_ContributionResult">ContributionResult</h2>
<!-- backwards compatibility -->
<a id="schemacontributionresult"></a>
<a id="schema_ContributionResult"></a>
<a id="tocScontributionresult"></a>
<a id="tocscontributionresult"></a>

```json
{
  "head": "string",
  "ids": [
    "string"
  ]
}

```

The outcome of a contribution — the new branch-table head and the appended claim ids.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|head|string|true|none|The new branch-table head id after the merge.|
|ids|[string]|true|none|Content-addressed ids of the contributed claims, in order.|

<h2 id="tocS_DevClockAdvance">DevClockAdvance</h2>
<!-- backwards compatibility -->
<a id="schemadevclockadvance"></a>
<a id="schema_DevClockAdvance"></a>
<a id="tocSdevclockadvance"></a>
<a id="tocsdevclockadvance"></a>

```json
{
  "time": "2019-08-24T14:15:22Z"
}

```

Requests the dev sequencer's clock advance to (at least) this instant. A
request older than the clock's current position is accepted as a no-op —
the clock only ever moves forward, since a merge's witnessed time
regressing would break every guarantee built on it.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|time|string(date-time)|true|none|The instant to advance to.|

<h2 id="tocS_DevClock">DevClock</h2>
<!-- backwards compatibility -->
<a id="schemadevclock"></a>
<a id="schema_DevClock"></a>
<a id="tocSdevclock"></a>
<a id="tocsdevclock"></a>

```json
{
  "time": "2019-08-24T14:15:22Z"
}

```

The dev sequencer's clock after the request — the later of what was asked
and what it already held.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|time|string(date-time)|true|none|none|

<h2 id="tocS_BranchHead">BranchHead</h2>
<!-- backwards compatibility -->
<a id="schemabranchhead"></a>
<a id="schema_BranchHead"></a>
<a id="tocSbranchhead"></a>
<a id="tocsbranchhead"></a>

```json
{
  "head": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|head|string|true|none|The content-addressed head claim id.|

<h2 id="tocS_BranchInfo">BranchInfo</h2>
<!-- backwards compatibility -->
<a id="schemabranchinfo"></a>
<a id="schema_BranchInfo"></a>
<a id="tocSbranchinfo"></a>
<a id="tocsbranchinfo"></a>

```json
{
  "name": "string",
  "head": "string",
  "height": 0,
  "updatedAt": "2019-08-24T14:15:22Z"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|true|none|The branch name, as the branch table holds it.|
|head|string|true|none|The content-addressed head claim id.|
|height|integer(int64)|true|none|The head claim's generation number (§4.1) — 0 on an initial node, else<br>1 + max over what it references. The depth of what the branch points at.|
|updatedAt|string(date-time)|true|none|The head claim's `created_at` — when the branch last moved. Soft: a<br>contributor writes it, so it is not the witnessed merge time.|

<h2 id="tocS_ArchiveInfo">ArchiveInfo</h2>
<!-- backwards compatibility -->
<a id="schemaarchiveinfo"></a>
<a id="schema_ArchiveInfo"></a>
<a id="tocSarchiveinfo"></a>
<a id="tocsarchiveinfo"></a>

```json
{
  "head": "string",
  "height": 0,
  "updatedAt": "2019-08-24T14:15:22Z",
  "branches": 0
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|head|string|true|none|The branch-table head id — the root of every `$archive`-scoped read, and the<br>only place this contract reports it.|
|height|integer(int64)|true|none|The branch-table head claim's generation number.|
|updatedAt|string(date-time)|true|none|The branch-table head's `created_at` — when the archive last moved.|
|branches|integer|true|none|How many branches the table holds.|

<h2 id="tocS_BranchEntry">BranchEntry</h2>
<!-- backwards compatibility -->
<a id="schemabranchentry"></a>
<a id="schema_BranchEntry"></a>
<a id="tocSbranchentry"></a>
<a id="tocsbranchentry"></a>

```json
{
  "name": "string",
  "head": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|true|none|The branch name, as the branch table holds it.|
|head|string|true|none|That branch's current head claim id.|

<h2 id="tocS_BranchList">BranchList</h2>
<!-- backwards compatibility -->
<a id="schemabranchlist"></a>
<a id="schema_BranchList"></a>
<a id="tocSbranchlist"></a>
<a id="tocsbranchlist"></a>

```json
{
  "branches": [
    {
      "name": "string",
      "head": "string"
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|branches|[[BranchEntry](#schemabranchentry)]|true|none|Every branch the branch table holds, from one archive snapshot — so the<br>heads are consistent with each other. Empty on an archive with no branches.|

<h2 id="tocS_Health">Health</h2>
<!-- backwards compatibility -->
<a id="schemahealth"></a>
<a id="schema_Health"></a>
<a id="tocShealth"></a>
<a id="tocshealth"></a>

```json
{
  "status": "ok",
  "version": "v1.19.2",
  "signer": "did:key:z6Mk..."
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|status|string|true|none|none|
|version|string|true|none|The server build answering this request. A release names itself exactly;<br>a build from a checkout names its revision, marked when the tree carried<br>uncommitted changes.|
|signer|string|false|none|The contributor identity this stack signs merges with.|

<h2 id="tocS_StorageLayer">StorageLayer</h2>
<!-- backwards compatibility -->
<a id="schemastoragelayer"></a>
<a id="schema_StorageLayer"></a>
<a id="tocSstoragelayer"></a>
<a id="tocsstoragelayer"></a>

```json
{
  "name": "string",
  "type": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|name|string|true|none|none|
|type|string|true|none|Adapter type (e.g. memory, filesystem, s3, redis, neo4j).|

<h2 id="tocS_StorageLayerList">StorageLayerList</h2>
<!-- backwards compatibility -->
<a id="schemastoragelayerlist"></a>
<a id="schema_StorageLayerList"></a>
<a id="tocSstoragelayerlist"></a>
<a id="tocsstoragelayerlist"></a>

```json
{
  "layers": [
    {
      "name": "string",
      "type": "string"
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|layers|[[StorageLayer](#schemastoragelayer)]|true|none|Read-through tiers, top (cache) to bottom (authoritative).|

<h2 id="tocS_VerificationConfig">VerificationConfig</h2>
<!-- backwards compatibility -->
<a id="schemaverificationconfig"></a>
<a id="schema_VerificationConfig"></a>
<a id="tocSverificationconfig"></a>
<a id="tocsverificationconfig"></a>

```json
{
  "closure": "string",
  "layer": "string",
  "depth": "completeness",
  "contentThreshold": 0
}

```

Parameters for a verification run — the same shape whether declared in the
stack config (scheduled) or posted ad-hoc. Depths are those of
core-verification.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|closure|string|true|none|Root of the closure to verify — a branch name or a claim id.|
|layer|string|false|none|Storage layer (by name) to read directly. Omit to read the composed<br>read-through view — which may mask the loss of an object on a deeper<br>layer, so name a layer to verify it without blind spots.|
|depth|string|false|none|completeness (a `has` sweep), record-correctness (recanonicalise and<br>recheck the id-chain and signatures), or full-content (also re-hash blobs).|
|contentThreshold|integer|false|none|For full-content, the max content size in bytes to re-read and re-hash<br>per claim; larger content is skipped this run. Omit to verify all content.|

#### Enumerated Values

|Property|Value|
|---|---|
|depth|completeness|
|depth|record-correctness|
|depth|full-content|

<h2 id="tocS_VerificationReport">VerificationReport</h2>
<!-- backwards compatibility -->
<a id="schemaverificationreport"></a>
<a id="schema_VerificationReport"></a>
<a id="tocSverificationreport"></a>
<a id="tocsverificationreport"></a>

```json
{
  "id": "string",
  "config": {
    "closure": "string",
    "layer": "string",
    "depth": "completeness",
    "contentThreshold": 0
  },
  "head": "string",
  "status": "running",
  "startedAt": "2019-08-24T14:15:22Z",
  "completedAt": "2019-08-24T14:15:22Z",
  "claimsChecked": 0,
  "bytesRead": 0,
  "ok": true,
  "failures": [
    {
      "id": "string",
      "mode": "corrupt-bytes",
      "layer": "string",
      "detail": "string"
    }
  ]
}

```

A point-in-time record of a verification run; embeds the config that produced it.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|true|none|none|
|config|[VerificationConfig](#schemaverificationconfig)|true|none|Parameters for a verification run — the same shape whether declared in the<br>stack config (scheduled) or posted ad-hoc. Depths are those of<br>core-verification.|
|head|string|true|none|The head id the run verified — the closure it pinned at start. When<br>`config.closure` names a branch, this is the branch's head at start time;<br>the report stays fixed to it even as the branch head moves on.|
|status|string|true|none|running (in progress), complete (finished on its own), stopped (cancelled<br>by an operator via the cancel action — partial findings kept), or error<br>(the run itself failed).|
|startedAt|string(date-time)|true|none|none|
|completedAt|string(date-time)|false|none|none|
|claimsChecked|integer|false|none|Claims checked so far; advances while `status` is `running`.|
|bytesRead|integer|false|none|Content bytes re-read so far; advances while `status` is `running`.|
|ok|boolean|true|none|True when no failures were found in this run.|
|failures|[[VerificationFailure](#schemaverificationfailure)]|false|none|The failures found, accumulating while `status` is `running`. Empty (with<br>`ok: true`) for an intact closure; a healthy archive verifies with none.|

#### Enumerated Values

|Property|Value|
|---|---|
|status|running|
|status|complete|
|status|stopped|
|status|error|

<h2 id="tocS_VerificationReportList">VerificationReportList</h2>
<!-- backwards compatibility -->
<a id="schemaverificationreportlist"></a>
<a id="schema_VerificationReportList"></a>
<a id="tocSverificationreportlist"></a>
<a id="tocsverificationreportlist"></a>

```json
{
  "reports": [
    {
      "id": "string",
      "config": {
        "closure": "string",
        "layer": "string",
        "depth": "completeness",
        "contentThreshold": 0
      },
      "head": "string",
      "status": "running",
      "startedAt": "2019-08-24T14:15:22Z",
      "completedAt": "2019-08-24T14:15:22Z",
      "claimsChecked": 0,
      "bytesRead": 0,
      "ok": true,
      "failures": [
        {
          "id": "string",
          "mode": "corrupt-bytes",
          "layer": "string",
          "detail": "string"
        }
      ]
    }
  ]
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|reports|[[VerificationReport](#schemaverificationreport)]|true|none|[A point-in-time record of a verification run; embeds the config that produced it.]|

<h2 id="tocS_VerificationFailure">VerificationFailure</h2>
<!-- backwards compatibility -->
<a id="schemaverificationfailure"></a>
<a id="schema_VerificationFailure"></a>
<a id="tocSverificationfailure"></a>
<a id="tocsverificationfailure"></a>

```json
{
  "id": "string",
  "mode": "corrupt-bytes",
  "layer": "string",
  "detail": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|id|string|true|none|The claim or object id that failed.|
|mode|string|true|none|corrupt-bytes — stored bytes don't match their hash (storage rot/loss;<br>self-heals via read-through if a deeper layer is intact).<br>invalid-content — the claim itself doesn't validate (e.g. bad signature);<br>unrepairable.|
|layer|string|true|none|The layer where the failure was observed.|
|detail|string|false|none|none|

#### Enumerated Values

|Property|Value|
|---|---|
|mode|corrupt-bytes|
|mode|invalid-content|

<h2 id="tocS_Error">Error</h2>
<!-- backwards compatibility -->
<a id="schemaerror"></a>
<a id="schema_Error"></a>
<a id="tocSerror"></a>
<a id="tocserror"></a>

```json
{
  "code": "string",
  "error": "string"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|code|string|true|none|Machine-readable failure category, stable across releases — one of<br>unauthenticated, forbidden, not_found, conflict, busy, invalid,<br>unimplemented, internal. Clients branch on this, not on the message.|
|error|string|true|none|Human-readable message. Carries no subject id, even on 403.|

<h2 id="tocS_Id">Id</h2>
<!-- backwards compatibility -->
<a id="schemaid"></a>
<a id="schema_Id"></a>
<a id="tocSid"></a>
<a id="tocsid"></a>

```json
"string"

```

A claim id: id(v) = Sign(H(S(v))) for a node, id(e) = H(S(e)) for an edge, carried as multibase base32 of the self-describing payload. The pattern fixes the multibase framing; whether the payload's multihash or multikey framing parses is the implementation's check.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|A claim id: id(v) = Sign(H(S(v))) for a node, id(e) = H(S(e)) for an edge, carried as multibase base32 of the self-describing payload. The pattern fixes the multibase framing; whether the payload's multihash or multikey framing parses is the implementation's check.|

<h2 id="tocS_TypeGlob">TypeGlob</h2>
<!-- backwards compatibility -->
<a id="schematypeglob"></a>
<a id="schema_TypeGlob"></a>
<a id="tocStypeglob"></a>
<a id="tocstypeglob"></a>

```json
"string"

```

A glob over class/sub, e.g. derivation/* or entity/person. A leading - excludes. Exclusion decides: a type matching an excluded pattern is refused whatever the included patterns say, and a list of exclusions alone admits every other type (R-QSTEPS).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|A glob over class/sub, e.g. derivation/* or entity/person. A leading - excludes. Exclusion decides: a type matching an excluded pattern is refused whatever the included patterns say, and a list of exclusions alone admits every other type (R-QSTEPS).|

<h2 id="tocS_PathStep">PathStep</h2>
<!-- backwards compatibility -->
<a id="schemapathstep"></a>
<a id="schema_PathStep"></a>
<a id="tocSpathstep"></a>
<a id="tocspathstep"></a>

```json
{
  "edges": [
    "string"
  ],
  "dir": "provenance",
  "min": 0,
  "max": 0,
  "nodes": [
    "string"
  ]
}

```

One section of a path. edges bounds the walk: every hop must follow an edge whose type is listed. nodes bounds the answer: a step yields a claim only when its node's type is listed, whatever the nodes it crossed to reach it. min and max bound the hops. A min above a bounded max is refused by the implementation — a JSON Schema cannot compare two sibling values (R-QSTEPS).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|edges|[[TypeGlob](#schematypeglob)]|false|none|Edge types every hop must match.|
|dir|string|false|none|provenance follows references outward, uses runs to the claims that cite this one, connections either way. Absent, provenance.|
|min|integer|false|none|Fewest hops. Absent, 1 — a step moves at least one hop. 0 also yields the starting set, carrying the frontier through alongside what lies beyond it.|
|max|integer|false|none|Most hops. 0, or absent, leaves the step unbounded: a step of at most zero hops would move nothing, so that reading has no use.|
|nodes|[[TypeGlob](#schematypeglob)]|false|none|Node types the step may yield.|

#### Enumerated Values

|Property|Value|
|---|---|
|dir|provenance|
|dir|uses|
|dir|connections|

<h2 id="tocS_Select">Select</h2>
<!-- backwards compatibility -->
<a id="schemaselect"></a>
<a id="schema_Select"></a>
<a id="tocSselect"></a>
<a id="tocsselect"></a>

```json
{
  "branch": "$universe",
  "head": "string",
  "claim": "string",
  "path": [
    {
      "edges": [
        "string"
      ],
      "dir": "provenance",
      "min": 0,
      "max": 0,
      "nodes": [
        "string"
      ]
    }
  ]
}

```

A generator in four independent parts: branch is the scope, head the closure read, claim where the walk starts, path the traversal. Scope and start are independent because a walk runs both ways — a uses step reaches the claims that cite the current one, which lie above it, so the closure decides a reverse step's answer.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|branch|string|true|none|The mandatory scope, and every scope names a graph: a branch name confines to that branch, $archive to the whole Ranke-Archive, $universe applies no confinement and is privileged. An empty value is refused (R-QSCOPE).|
|head|[Id](#schemaid)|false|none|The closure read: the query sees the intersection of the scope's graph and closure(head), so a head narrows a query. Required under $universe, which confines nothing and so offers no head to fall back on, and may name any claim the Universe holds there; optional under every other scope, where the scope's own head serves (R-QHEAD).|
|claim|[Id](#schemaid)|false|none|Anchors the frontier at the single claim it names, which must lie inside the closure. Absent, the frontier is every claim in the closure and the path is unanchored (R-QANCHOR).|
|path|[[PathStep](#schemapathstep)]|false|none|The traversal: a sequence of steps over frontiers, each frontier a set of claims. Each step's yield is the frontier the next starts from, and the no-repeat rule holds within a step and resets at each boundary, so membership is all a frontier carries (R-QFRONTIER). Absent, the generator returns the full outward closure of the frontier (R-QSTEPS).|

<h2 id="tocS_Where">Where</h2>
<!-- backwards compatibility -->
<a id="schemawhere"></a>
<a id="schema_Where"></a>
<a id="tocSwhere"></a>
<a id="tocswhere"></a>

```json
{
  "and": [
    {
      "and": []
    }
  ]
}

```

A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT).

### Properties

oneOf

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|object|false|none|none|
|» and|[[Where](#schemawhere)]|true|none|[A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT).]|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|object|false|none|none|
|» or|[[Where](#schemawhere)]|true|none|[A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT).]|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|object|false|none|none|
|» not|[Where](#schemawhere)|true|none|A boolean tree. Each node is exactly one of the and / or / not combinators over sub-trees, or a leaf naming a field and its test. Within a where, or is boolean; across generators it unions whole result sets. A leaf may name any field a claim carries, height included (V-HEIGHT).|

xor

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|object|false|none|A leaf: one field, one comparison.|
|» field|string|true|none|The field tested — any field a claim carries, height included.|
|» test|[Comparison](#schemacomparison)|true|none|One operator applied to one field. eq, ne, lt, le, gt and ge take a value, in a set, glob a shell-style wildcard. Exactly one is present.|

<h2 id="tocS_Value">Value</h2>
<!-- backwards compatibility -->
<a id="schemavalue"></a>
<a id="schema_Value"></a>
<a id="tocSvalue"></a>
<a id="tocsvalue"></a>

```json
null

```

A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.

### Properties

*None*

<h2 id="tocS_Comparison">Comparison</h2>
<!-- backwards compatibility -->
<a id="schemacomparison"></a>
<a id="schema_Comparison"></a>
<a id="tocScomparison"></a>
<a id="tocscomparison"></a>

```json
{
  "eq": null
}

```

One operator applied to one field. eq, ne, lt, le, gt and ge take a value, in a set, glob a shell-style wildcard. Exactly one is present.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|eq|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|ne|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|lt|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|le|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|gt|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|ge|[Value](#schemavalue)|false|none|A value a comparison tests against. Where it tests a time it MUST be a V-TIME timestamp or an EDTF Level 1 value, and anything else is rejected rather than coerced (R-QTIMEOP); otherwise how two values compare is the engine's.|
|in|[[Value](#schemavalue)]|false|none|Set membership.|
|glob|string|false|none|Shell-style wildcard.|

<h2 id="tocS_OutputContent">OutputContent</h2>
<!-- backwards compatibility -->
<a id="schemaoutputcontent"></a>
<a id="schema_OutputContent"></a>
<a id="tocSoutputcontent"></a>
<a id="tocsoutputcontent"></a>

```json
{
  "max": 0,
  "overflow": "cutoff"
}

```

Inline content per claim. Absent, no content is inlined (R-QCONTENT).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|max|integer|true|none|Cap in bytes on the content inlined per claim; 0 inlines every claim's content in full.|
|overflow|string|false|none|What becomes of content past the cap: cutoff inlines the bytes up to it, omit inlines whole values only. Absent, omit. A claim keeps every field it carries either way (R-QCONTENT).|

#### Enumerated Values

|Property|Value|
|---|---|
|overflow|cutoff|
|overflow|omit|

<h2 id="tocS_Output">Output</h2>
<!-- backwards compatibility -->
<a id="schemaoutput"></a>
<a id="schema_Output"></a>
<a id="tocSoutput"></a>
<a id="tocsoutput"></a>

```json
{
  "shape": "single",
  "detail": "id",
  "form": "original",
  "content": {
    "max": 0,
    "overflow": "cutoff"
  },
  "encoding": "json"
}

```

Shapes each result along orthogonal axes. detail: claims with form: original and encoding: cbor reproduces the canonical serialization S(v) a claim's id is computed over, and is the only output form directly verifiable against that id (R-QCANON).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|shape|string|false|none|single yields the reached endpoints, one element each; path yields routes, each running outward from the frontier claim its walk began at (R-QSHAPE).|
|detail|string|false|none|What each element carries: id (the id alone), claims (a record the engine assembles, shaped by form and content), or envelope (the stored bytes copied out whole, the only output a client can hash against an id; form and content do not apply, and both form: materialized and encoding: json are rejected with it). Under shape: path it applies to every claim in the route (R-QDETAIL, R-QCANON).|
|form|string|false|none|Which field values a claim carries: original as written, a diff-overlaid claim's delta; materialized with any contribution/diff chain resolved over the predecessor it references, recursively to a base claim. A property of the values, hence orthogonal to detail and encoding (R-QFORM).|
|content|[OutputContent](#schemaoutputcontent)|false|none|Inline content per claim. Absent, no content is inlined (R-QCONTENT).|
|encoding|string|false|none|json is text with content base64-encoded, cbor is binary; the same information either way (R-QENCODING).|

#### Enumerated Values

|Property|Value|
|---|---|
|shape|single|
|shape|path|
|detail|id|
|detail|claims|
|detail|envelope|
|form|original|
|form|materialized|
|encoding|json|
|encoding|cbor|

<h2 id="tocS_OrderKey">OrderKey</h2>
<!-- backwards compatibility -->
<a id="schemaorderkey"></a>
<a id="schema_OrderKey"></a>
<a id="tocSorderkey"></a>
<a id="tocsorderkey"></a>

```json
{
  "field": "string",
  "compare": "numeric",
  "dir": "asc"
}

```

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|field|string|true|none|The field sorted on — any field a claim carries, height included.|
|compare|string|false|none|How the values compare (R-QSORT). temporal reads each value as the span of time it denotes and orders by that span's midpoint in nanoseconds, so EDTF dates and instants compare on one axis (R-QTEMPORAL).|
|dir|string|false|none|Sort direction (R-QSORT).|

#### Enumerated Values

|Property|Value|
|---|---|
|compare|numeric|
|compare|lexical|
|compare|temporal|
|dir|asc|
|dir|desc|

<h2 id="tocS_Order">Order</h2>
<!-- backwards compatibility -->
<a id="schemaorder"></a>
<a id="schema_Order"></a>
<a id="tocSorder"></a>
<a id="tocsorder"></a>

```json
[
  {
    "field": "string",
    "compare": "numeric",
    "dir": "asc"
  }
]

```

Sort keys applied in priority order. Claims lacking a key's field sort last, and the archive's natural (created_at, id) order breaks any remaining ties, so the sort always resolves to a total order (R-QSORT).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|[[OrderKey](#schemaorderkey)]|false|none|Sort keys applied in priority order. Claims lacking a key's field sort last, and the archive's natural (created_at, id) order breaks any remaining ties, so the sort always resolves to a total order (R-QSORT).|

<h2 id="tocS_Duration">Duration</h2>
<!-- backwards compatibility -->
<a id="schemaduration"></a>
<a id="schema_Duration"></a>
<a id="tocSduration"></a>
<a id="tocsduration"></a>

```json
"string"

```

A duration as a decimal sequence with unit suffixes — ns, us, ms, s, m, h — e.g. 5s or 1m30s. The bare 0 means unbounded.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|*anonymous*|string|false|none|A duration as a decimal sequence with unit suffixes — ns, us, ms, s, m, h — e.g. 5s or 1m30s. The bare 0 means unbounded.|

<h2 id="tocS_Limit">Limit</h2>
<!-- backwards compatibility -->
<a id="schemalimit"></a>
<a id="schema_Limit"></a>
<a id="tocSlimit"></a>
<a id="tocslimit"></a>

```json
{
  "results": 0,
  "time": "string"
}

```

Bounds the read. A read cut short by either bound is a complete answer to the query as bounded, not an error (R-QLIMIT).

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|results|integer|false|none|Caps the claim count; 0 is unbounded.|
|time|[Duration](#schemaduration)|false|none|The execution budget; 0 is unbounded.|

<h2 id="tocS_Execution">Execution</h2>
<!-- backwards compatibility -->
<a id="schemaexecution"></a>
<a id="schema_Execution"></a>
<a id="tocSexecution"></a>
<a id="tocsexecution"></a>

```json
{
  "layer": "string",
  "report": "info"
}

```

Where the query runs and how it reports on itself. These controls reach execution and diagnostics, and never the result set.

### Properties

|Name|Type|Required|Restrictions|Description|
|---|---|---|---|---|
|layer|string|false|none|Pins the query to one named storage or execution layer; absent, the backend chooses by capability.|
|report|string|false|none|Report verbosity: info gives high-level stages, debug routing and translation, trace per-claim detail. Set, and only then, the stream carries one final report record after the last element, typed distinctly from result claims (R-QREPORT).|

#### Enumerated Values

|Property|Value|
|---|---|
|report|info|
|report|debug|
|report|trace|

