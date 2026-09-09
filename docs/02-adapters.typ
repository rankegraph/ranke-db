#import "vocabulary.typ": *

= The ports the server drives <ch:adapters>

Four sections name backends the server reaches out to: `storage`, `sequencer`,
`signer` and `vault`. Each is an object naming its backend under `"type"` and
carrying that backend's fields beside it. Everything below is written from the
configuration's side — the field to write, what it selects, and what happens
when it is absent.

== `storage` — where claims live <sec:storage>

The storage section describes the #gls("universe") as a tree: composite types
hold other descriptors, leaf types name a technology. The whole section is one
descriptor, so the simplest storage is a single leaf, and the usual shape is a
`stack` of them.

=== Composites <sec:storage-composites>

#item("stack")[
  `layers`, an array of descriptors read in order, first entry first. A read
  tries each layer in turn and a miss is repaired from the layer below, which
  is what makes a cache in front of a slow backing store useful. An empty array
  is refused.
]

#item("partition")[
  `shards`, an array of descriptors. Content is routed to one shard by its
  #gls("id"), so the shards divide the data rather than duplicate it. An empty
  array is refused.
]

#warning[A partition's shard count is fixed at launch and the routing depends
on it. Adding a shard later changes where every id lives, so the archive has to
be rewritten rather than extended.]

Either composite may hold the other, and either may hold a leaf, to whatever
depth the deployment wants.

=== Leaves <sec:storage-leaves>

#item("mem")[
  Keeps everything in memory. No fields. The process exit is the end of the
  data — good for trying things out and for tests.
]

#item("minimal")[
  The reference in-memory store: the smallest thing that satisfies the port,
  kept as an illustration and deliberately not optimised. No fields. Reach for
  `mem` instead.
]

#item("fs")[
  `dir`, a directory path. The store is files under it, created as needed.
]

#item("sqlite")[
  `dsn`, a SQLite data-source name — a file path, or `:memory:`.
]

#item("s3")[
  `bucket` and `region`, plus `accessKeyId` and `secretAccessKey`, all four
  required. Credentials come from this configuration alone (as literals or
  through `env(...)`/`vault(...)`) and never from an ambient credential chain,
  so one file accounts for a deployment's secrets. `endpoint` and
  `usePathStyle` (`"true"`) point the same backend at an S3-compatible service
  such as MinIO.
]

#item("redis")[
  `addr` (`host:port`), with optional `password` and `db`, the numeric database
  index.
]

#item("neo4j")[
  `uri`, required. `username` and `password` go together — give neither and the
  driver connects without authentication, give one and the other becomes
  required. `database` selects a database other than the default.
]

=== Fields every descriptor accepts <sec:storage-common>

#item("name")[
  A label for this layer, used wherever the server reports layers and by a
  query that pins itself to one. Absent, the layer is named after its type; two
  layers of one name are distinguished by position.
]

#item("mode")[
  The storage tier this layer is expected to serve — `authoritative`, `eager`,
  `background` or `lazy`. An assertion rather than a setting: a backend serves
  the tier it serves, and a `mode` that disagrees fails the launch.
]

#item("maxContentSize")[
  The content cap this layer is expected to hold to, in bytes. An assertion in
  the same sense, and against the cap the backend reports.
]

Write the two assertions where a deployment wants a mismatch caught, and leave
them out otherwise.

#example[
#listing[
```json
"storage": {
  "type": "stack",
  "layers": [
    {"type": "mem", "name": "cache"},
    {"type": "fs", "name": "archive", "dir": "/var/lib/rankedb/blobs"}
  ]
}
```
]
]

== `sequencer` — how the head advances <sec:sequencer>

The sequencer moves the #gls("branch-table") head as contributions merge, and
keeps the previous heads so a deployment can look back at where the archive
stood.

#item("type")[
  `"concurrent"` or `"dev"`. `concurrent` is the one to deploy: it prepares
  contributions in parallel and folds a whole batch into one advance of the
  head. `dev` is the blocking reference sequencer — one contribution at a time,
  callers queueing behind it — for tests and development, and it is what
  `--dev` requires.
]

#item("seed")[
  Names the #gls("bookmark") list this instance advances. The same seed reaches
  the same bookmarks; a different seed starts a different list. Exclusive with
  `bookmark`, and one of the two is required.
]

#item("bookmark")[
  The id of one #gls("bookmark"), naming its list the other way about: the
  record there carries the #gls("bookmark-seed") every entry in the list holds.
  This is the way back in when the earliest entries were lost. Exclusive with
  `seed`, and one of the two is required.
]

#item("founder")[
  The PEM public key of the archive's first #gls("contributor"), under which a
  launch founds an archive that does not exist yet. Optional: leaving it out
  means founding by hand instead.
]

A seed is a name, not a secret. Its entropy keeps lists that nobody coordinated
apart, which is why `V-BMENV` states 128 bits as a SHOULD, so a distinctive
label serves wherever you own the store.

The #gls("bookmark-store") is the #gls("universe")'s own, so the list inherits
the layering, replication and backup of the storage beneath it. A layer that is
a rebuildable projection holds none, the head being lost with the reindex; a
stack holds them when one of its authoritative layers does, and a partition
replicates the list to every shard, so it needs all of them to.

#warning[
  Losing the seed and every bookmark id together loses the archive: a
  #gls("bookmark-store") is how a moving head is found at all, so the claims
  stay intact in the #gls("universe") with nothing left pointing at them. Keep
  the bookmark id a launch reports.
]

#example[
#listing[
```json
"sequencer": {
  "type": "concurrent",
  "seed": "production-archive",
  "founder": "env(RANKE_FOUNDER_PUBKEY)"
}
```
]
]

=== Founding an archive <sec:founding>

An archive comes into being once. A sequencer whose #gls("bookmark") list is
empty holds no archive yet and refuses every read and every write until one is
founded, which writes the sequencer's own initial claim, the first
#gls("contributor") under it, the empty #gls("branch-table") and its first
bookmark.

A launch resolves that state before it serves. With `founder` set it founds and
carries on, reporting the first contributor, the head and the bookmark in its
log. Without it the launch refuses, naming what is missing, rather than serving
an archive that is not there.

#listing[
```sh
ranke-db found config.json first-contributor.pub.pem
```
]

The command founds and exits, reporting the same three ids. An archive that
already exists is reported as such and left alone, so provisioning may call it
ahead of every launch. Only the public half of the founding key is ever given
to the server: whatever contributes under that identity keeps the rest.

== `signer` — the identity merges are attested with <sec:signer>

This is the server's own key, and it signs merges. It never signs a
#gls("contributor")'s #glspl("claim"): those arrive signed already, by the key
of whoever made them.

#item("inmemory")[
  `key`, an Ed25519 private key as PKCS\#8 PEM, required — this backend mints
  nothing and needs to be given one. Writing it as `env(...)`/`vault(...)`
  keeps the key out of the file.
]

#item("openbao")[
  Signing through an OpenBao Transit engine, so the key stays in the vault.
  `address` (server URL) and `token` are required; `mount` is the Transit mount
  path, `"transit"` by default; `key` names the Transit key.
]

#warning[Changing this key changes the identity every later merge is attested
under. Pick one for a deployment and keep it.]

== `vault` — where `vault(...)` reads from <sec:vault>

Configure this section only if some value in the document is written as
`vault(REF)`. It is built the first time such a value is read, and its own
fields may use `env(...)` — a vault cannot bootstrap itself out of the store it
is the way to.

#item("openbao")[
  `address` and `token`, both required; `mount`, the KV v2 mount path,
  `"secret"` by default. A `vault(REF)` reference names the path within that
  mount.
]

#item("azure")[
  `url`, an Azure Key Vault URI. The backend accepts its configuration and
  refuses every read — secret resolution is not wired yet, so a deployment
  cannot rely on it.
]

#example[
#listing[
```json
"vault":  {"type": "openbao", "address": "https://bao.internal:8200",
           "token": "env(BAO_TOKEN)", "mount": "secret"},
"signer": {"type": "inmemory", "key": "vault(rankedb/signing-key)"}
```
]
]
