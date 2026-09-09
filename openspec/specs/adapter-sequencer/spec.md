# adapter-sequencer Specification

## Purpose
The sequencer port advances a Ranke-Archive's head `k → k′` as contributions merge, and
records each advance as a **bookmark** so the moving head stays findable. This capability
defines the port contract; the merge pipeline that drives it (verification, timestamping,
atomic advance) is `core-contribution`.

A bookmark is a signed record of three fields — the seed `s` fixed once per list, the
index `i`, and the head id `k` it records — stored in `𝒰_hist` under `id_seq(i, s)`
(foundation paper §Bookmarks). `𝒰_hist` belongs to the Universe and is reached through
it, so a bookmark list inherits the layering, replication and backup of the storage
beneath it (RankeDB paper §Sequencer).

## Requirements

### Requirement: The bookmark list is named by a seed or by one of its entries
The system SHALL take exactly one of a seed or a bookmark id, and SHALL refuse a
configuration giving both or neither. A seed names a list from index 0; a bookmark id
names a list through a surviving entry, whose record yields the seed every bookmark in
that list carries, which is the way back in when the earliest entries were lost.

A seed is a name and carries no security value: entropy keeps uncoordinated lists apart
and nothing more, so any non-empty value SHALL be accepted (`V-BMENV` states 128 bits as
a SHOULD).

#### Scenario: The same seed reaches the same list
- **WHEN** two launches over one Universe are given one seed
- **THEN** the second reads the head the first recorded

#### Scenario: A bookmark id reopens a list without its seed
- **WHEN** a launch is given the id of a surviving bookmark instead of a seed
- **THEN** the list is opened from the seed that record carries

#### Scenario: Naming the list twice, or not at all, is refused
- **WHEN** a configuration carries both a seed and a bookmark id, or neither
- **THEN** the sequencer refuses to build, naming the two keys

### Requirement: An archive is founded once, explicitly
The system SHALL create an archive only through an explicit founding step, which writes
the sequencer's initial claim, the first contributor carrying the given public key, the
empty branch table `k₀`, and its bookmark. A second founding SHALL be refused, and every
other operation SHALL be refused until the first succeeds.

Only the public half of the founding key reaches the server; the private half stays with
whatever contributes under that identity.

#### Scenario: An unfounded archive answers nothing else
- **WHEN** a read reaches a sequencer whose bookmark list is empty
- **THEN** it is refused as an archive awaiting its first contributor, not as a fault

#### Scenario: Founding twice is refused
- **WHEN** an archive that already exists is asked to found again
- **THEN** the request is refused and the existing archive is left as it is

### Requirement: A restart reopens the archive its bookmark list records
The system SHALL take its state, branch heads included, from the bookmark list it opens,
and SHALL write nothing while doing so. Construction that founded unconditionally would
publish a second and empty head above the real archive, leaving it unreachable while its
claims sat intact in the Universe.

#### Scenario: A relaunch serves the recorded head
- **WHEN** a server is restarted over the same storage and the same seed
- **THEN** it serves the head and the branches the bookmark list records

#### Scenario: Opening an archive writes no bookmark
- **WHEN** a server is restarted over a list holding `n` bookmarks
- **THEN** the list still holds `n` bookmarks

### Requirement: A layer that cannot hold a bookmark store is refused
The system SHALL refuse a storage tree that cannot hold `𝒰_hist`, naming the configured
layers, because an archive whose advances cannot be recorded is unreachable the moment it
moves. A backend that is a rebuildable projection holds none, the head being lost with
the reindex. A stack holds them when one of its authoritative layers does; a partition
replicates the list to every shard and so needs all of them to.

#### Scenario: A tree with no bookmark-capable layer fails at launch
- **WHEN** a sequencer is configured over storage reporting no bookmark capability
- **THEN** the launch fails, naming the configured layers and the composition rule

### Requirement: The sequencer is a graph-registered contributor
The system SHALL register the sequencer in the graph as a contributor holding a private
key, so it can sign every branch-table claim and every bookmark it writes; the key and
the signing are supplied by the vault and signer ports (see `adapter-vault`,
`adapter-signer`, `core-contribution`).

#### Scenario: The sequencer signs with its configured identity
- **WHEN** the sequencer creates a branch-table claim
- **THEN** it signs the claim with its registered contributor key obtained via the signer
