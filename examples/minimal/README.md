# Minimal example

The smallest launchable `ranke-db` stack: in-memory storage, no auth, an
in-process signing identity.

The config is **secret-free** — `signer.key` is `env(RANKE_SIGNER_KEY)`, so the
file itself holds no key and is safe to commit. The signing identity is supplied
at launch from the environment (a real deployment would point this at a
`vault(...)` reference or an inline key in an age-encrypted config).

## Run it

From the repo root:

```sh
make dev
```

That builds the binary, mints a throwaway Ed25519 signing key, and launches this
config. Point it at another config with `make dev DEV_CONFIG=path/to/config.json`.

By hand, if you prefer — the signer key is an Ed25519 private key in PKCS#8 PEM
form, and the address comes from the config's `endpoints` section, not a flag:

```sh
export RANKE_SIGNER_KEY="$(openssl genpkey -algorithm ed25519)"
ranke-db run examples/minimal/config.json
```

## Seed it

An empty archive is hard to look at, so `bin/generator` fills one. It is a **client**,
not a server feature: a contributor is an application-held key, so the generator signs
its own claims and sends them to `POST /contribute` the way any application would.

```sh
make dev SEED=example                 # serve, and seed as soon as /health answers
make dev SEED=chain                   # 20 contributions × 10 claims (CONTRIBUTIONS=, CLAIMS=)
make seed SEED_URL=http://host:8080   # seed a server that is already up
```

`example` is the smallest graph with real provenance: two `source/note` claims, a
`derivation/extraction` citing both, an `entity/person` distilled from that.
`chain` grows one contribution at a time, each citing what came before, so heights
climb and the branch table accumulates a revision per contribution — the shape worth
testing a client against.

Seeding with `make dev` rather than in a second run matters here: this stack keeps
nothing on disk, so an archive only exists while the process serving it is up. Against
a persistent storage, seed once with `make seed` and relaunch as often as you like —
a restart reopens the archive its bookmark list records.

The fixture identity is derived from its name (`--as`, default `dev`) and the clock is
pinned, so the same command always produces the same claim ids — re-seeding merges
nothing new.

`make smoke` runs the whole cycle as a self-test: launch, health-check, seed over the
API, read the graph back, shut down.

## What a running instance answers today

The stack assembles fully — storage, sequencer, signer, and two REST endpoints, one on
a port and one on a socket — and `verify --level connect` proves it. The read and
write surface is live:

| Route | Answers |
|---|---|
| `GET /health` | the server's signing identity |
| `GET /branches` | every branch by name and head — start here, no branch name needed |
| `GET /branches/{branch}/head`, `…/claims/{id}` | the branch head, a claim in its closure |
| `GET /archive/claims/{id}`, `GET /universe/claims/{id}` | a claim by the scope it is read in |
| `POST /contribute` | merges a contribution, returns the new head and the ids |
| `POST /query` | the branch's closure, as `json` or `cbor` |
| `GET /system/layers`, `POST /system/verifications` | storage introspection, verification |

### Reaching it from a browser

`allowedOrigins` on the transport admits the explorer's dev origins — vite's `5173` and its
preview `4173` — so `make dev` here and `make -C frontend dev` there work together with no
proxy. It is a comma-separated list and `*` admits any; omitting it leaves the instance
unreachable from any page but its own origin, which is the right default for a server nobody
browses. Admitting an origin grants it nothing: every request still authenticates and is
authorized as before.

### The admin endpoint, on a socket

The second endpoint binds `unix://./run/admin.sock` instead of a port, admitting
`admin` rather than `ops`. `mode: "0660"` makes group membership the admission check; the
default (`mode` absent) admits only the user the server runs as. Either way, the
socket's containing directory carries the boundary too, for the instant between bind
and chmod — create it closed before launch:

```sh
mkdir -m 0700 run
```

Reach it from a workstation by forwarding a local port over ssh (OpenSSH 6.7+; give
the socket's absolute path, since a forwarded path is resolved on the remote host, not
against the server's own working directory):

```sh
ssh -L 8080:/run/rankedb/admin.sock host
```

and open the Ranke Explorer at `http://127.0.0.1:8080` — it never learns the instance
is behind a socket rather than a port.

### The sequencer section

`"type": "dev"` binds ranke-go's serial reference writer, one contribution at a time —
right for a dev server. `"concurrent"` selects the writer that prepares contributions
in parallel and folds a batch into one head advance.

`"seed"` names the bookmark list this instance advances. A bookmark records a head id
in 𝒰_hist, which the storage beneath holds, so the list inherits that storage's
layering and replication. The seed is a **name, not a secret**: the same seed reaches
the same bookmarks, a different one starts a different list. Its entropy only keeps
lists nobody coordinated apart, so a distinctive label serves whenever you own the
store. Use `"bookmark": "<id>"` instead to reopen a list whose earliest entries were
lost — any surviving entry carries the seed the whole list shares.

`"found"` is what a launch brings the archive into being with. `branch` names the
branch it begins on, and `pubkey` is the PEM **public** key of its first contributor;
the private half stays with whatever application contributes under that identity.

The branch matters: founding binds the first contributor to it, and that binding is
what makes the contributor reachable at all. An archive founded without one holds a
contributor claim nothing references, so nobody can ever write to it.

Leave `found.pubkey` out and the launch refuses to serve an archive that does not
exist yet, naming what is missing — found it by hand instead:

```sh
ranke-db found examples/minimal/config.json first-contributor.pub.pem
```

which reports the first contributor, the head and a bookmark, then exits. Record the
bookmark: with the seed lost, it is the only way back to the archive.
