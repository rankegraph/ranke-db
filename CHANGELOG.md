# Changelog

What each release changed for someone depending on this repository.

## Unreleased

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
