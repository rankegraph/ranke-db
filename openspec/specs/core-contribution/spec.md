# core-contribution Specification

## Purpose
A contribution is how new claims enter a Ranke-Archive. The core receives a client's
claims, verifies them, checks their timestamps, persists them, and merges them
atomically by advancing the branch-table head (BTH) through the sequencer. This
capability defines that pipeline: the verify-on-add gate, the timestamp rules
enforced at merge, atomic batch absorption, persistence-before-advance with
rollback, and the server's role as the contributor that signs each merge.

## Requirements

### Requirement: Contributions are verified before they enter
The system SHALL verify every submitted claim before the BTH advances, and SHALL
admit only verified claims. Verification recomputes each claim's id-chain and
signatures and its content hashes (see `core-verification`). A contribution
containing any unverifiable claim SHALL be rejected in full.

#### Scenario: An unverifiable claim is rejected
- **WHEN** a contribution contains a claim whose recomputed id or signature does not match
- **THEN** the contribution is rejected and the BTH does not advance

#### Scenario: Verification is a gate on entry
- **WHEN** a contribution is accepted
- **THEN** every claim it added was verified as it entered

### Requirement: created_at is monotonic and ceilinged at merge
The system SHALL reject a claim dated earlier than any claim it references
(monotonicity) and a claim dated later than the server's clock at merge time
(ceiling). The system SHALL NOT judge the plausibility of a creation date beyond
these two checks; content-domain plausibility is the client application's concern.

#### Scenario: A claim earlier than its reference is rejected
- **WHEN** a submitted claim's `created_at` precedes the `created_at` of a claim it references
- **THEN** the contribution is rejected

#### Scenario: A future-dated claim is rejected
- **WHEN** a submitted claim's `created_at` is later than the server clock at merge
- **THEN** the contribution is rejected

### Requirement: Claims persist before the BTH advances
The system SHALL write a contribution's claims into the storage stack, guaranteeing
their persistence, before advancing the BTH to the new branch table. If a storage
write fails, the BTH SHALL NOT advance, so no head ever points at a claim that
failed to persist.

#### Scenario: A failed storage write does not advance the head
- **WHEN** a claim in a contribution fails to persist to the storage stack
- **THEN** the BTH is left pointing at the prior branch table and the contribution fails

### Requirement: The sequencer merges atomically by advancing the BTH
The system SHALL, on accepting a contribution, create a new `contribution/branches`
branch-table claim that references the newly added claim(s) and, as its predecessor,
the branch table the BTH currently points at, then advance the BTH to the new claim.
A single merge step SHALL absorb any number of claims, each carrying its full
referenced closure into the archive.

#### Scenario: A batch merges in one head advance
- **WHEN** a contribution submits N verified claims
- **THEN** the sequencer records them under a single new branch table and advances the BTH once

#### Scenario: The new branch table references its predecessor
- **WHEN** a new branch table is created
- **THEN** it references the previous branch table as provenance, extending the BTH history

### Requirement: The server signs each merge as a registered contributor
The system SHALL sign every branch-table claim it creates with the server's signing
identity, obtained through the signer and vault ports, and that identity SHALL be
registered in the graph as a contributor. A merge without a valid contributor
signature SHALL NOT be produced.

#### Scenario: A branch table carries a valid server signature
- **WHEN** the sequencer creates a branch-table claim
- **THEN** the claim carries the server's `contribution/contributor` and a signature that verifies against it

### Requirement: Claims have a total order
The system SHALL treat the pair `(created_at, id)` as a total order over all claims
in the archive, used by reads for ordering and pagination (see `core-query`).

#### Scenario: Two claims are strictly ordered
- **WHEN** two distinct claims are compared
- **THEN** `(created_at, id)` orders them deterministically

### Requirement: Verification may be distributed while the sequencer stays central
The system SHALL keep the sequencer a single authoritative instance while permitting
the verification workload to be distributed across trusted verification servers.
Such servers verify submitted claims, feed them into the persistence layers, and
bundle them under a single signed claim attesting to the verification performed; the
sequencer recognises the trusted verifiers' signatures and commits the bundled
claims.

#### Scenario: A trusted verifier bundles a bulk contribution
- **WHEN** a trusted verification server submits a bundle of pre-verified claims under its attesting signature
- **THEN** the central sequencer accepts the bundle on the strength of that signature and advances the BTH once
