#import "vocabulary.typ": *

= Running an instance <ch:running>

A `ranke-db` instance is one binary and one configuration file. Put the binary
somewhere, write the file, and launch: the configuration says where the data
lives, which addresses are served, and who may act on the archive.

#listing[
```sh
ranke-db run config.json
```
]

That serves until you stop it (Ctrl-C, or a `SIGTERM`). A configuration is read
once, at launch: an instance is never reconfigured while it runs, so the
administrative cycle is edit, run, observe, stop, edit again. Two instances that
differ in any way are two processes with two files.

#note[The configuration may come from standard input instead, written as `-`:
`ranke-db run -`.]

== Checking a configuration before launching <sec:verify>

`ranke-db verify config.json` answers `config ok` or the first problem it
finds. It checks to one of three depths, and each depth includes the ones
before it.

#item("--level syntax")[
  The default. Parses the document and shape-checks it: unknown top-level
  sections, malformed grants, an `admit` list naming an account that does not
  exist, and a malformed endpoint address all fail here. It touches neither the
  environment nor the network, so it is the check to run in a pipeline that
  holds no secrets.
]

#item("--level resolve")[
  Also expands every `env(...)` and `vault(...)` reference, so a missing
  variable or an unreadable secret fails before launch. This dials the vault if
  — and only if — some value references it.
]

#item("--level connect")[
  Also assembles every adapter the file names and discards them, so an
  unreachable database or a bad credential surfaces here rather than
  mid-launch.
]

== The shape of the document <sec:document>

The configuration is one JSON object with six top-level sections, and no others
— an unrecognised section is refused rather than ignored, so a misspelling
never passes silently.

#item("storage")[
  Where #glspl("claim") are kept — the #gls("universe"). See @sec:storage.
]

#item("sequencer")[
  What advances the #gls("branch-table") head as contributions merge, and
  records each advance as a #gls("bookmark"). See @sec:sequencer.
]

#item("signer")[
  The signing identity this server attests merges with. See @sec:signer.
]

#item("vault")[
  The secret store `vault(...)` references read from. Optional; needed only if
  some value uses one. See @sec:vault.
]

#item("accounts")[
  Who may do what, as policy of this server. See @ch:access.
]

#item("endpoints")[
  The addresses served, each with its own authentication and its own admitted
  accounts. See @ch:endpoints.
]

The first four are the ports the server drives; the last two drive it. Each
section names a backend under `"type"` and carries that backend's own fields
beside it.

A section left out yields no adapter, which is why `verify --level syntax`
passes on a partial document: a file worth launching names at least `storage`,
`sequencer`, `signer` and one endpoint.

== Values, secrets, and where they come from <sec:secrets>

Any string value in any section may be written as a delegation instead of a
literal.

#item("env(NAME)")[
  Reads the process environment.
]

#item("vault(REF)")[
  Reads the secret store the `vault` section configures.
]

#example[
#listing[
```json
"key":   "env(RANKE_SIGNER_KEY)",
"token": "vault(kv/rankedb/openbao-token)"
```
]
]

The match is whole-value: a delegation is the entire string or it is not one at
all, so no secret hides inside a longer value, and `"addr": "env(PORT):8080"`
is a literal address rather than a substitution. A reference that resolves to
nothing is an error — resolution fails loudly rather than yield an empty
secret.

Resolution happens at use. The environment is always available; the vault is
built from its own section the first time some other value asks for a secret,
which is why that section may use `env(...)` and never `vault(...)`. Nothing
resolves during a `syntax` check.

== Encrypting the whole file <sec:age>

A configuration may be stored encrypted with age, passphrase-based, and both
`run` and `verify` accept it as it stands: the binary detects the encryption
and decrypts before parsing.

#item("--age-key prompt|stdin|env:VAR|file:path")[
  Where the passphrase comes from. `prompt` asks on the terminal.
]

#example[
#listing[
```sh
ranke-db run --age-key prompt        config.json.age
ranke-db run --age-key env:AGE_KEY   config.json.age
ranke-db run --age-key file:/run/key config.json.age
```
]
]

Reading both the configuration and the passphrase from standard input is
refused, since the two cannot be told apart.

Encryption covers the file as a whole, so it protects the values written
literally in it. A deployment that keeps its secrets in the environment or a
vault gets the same protection without the passphrase, and the two combine.

== The development clock <sec:dev>

#item("--dev")[
  Mounts `POST /dev/clock`, which steers the time the #gls("sequencer") stamps
  merges with instead of following real time. It requires `sequencer.type` to
  be `"dev"`, and exists to make a test suite's timeline reproducible.
]

#warning[Never enable `--dev` on a deployment anyone else can reach. A caller
who can move the clock chooses what a merge witnesses, and the witnessed
window is the one timestamp an archive treats as hard.]
