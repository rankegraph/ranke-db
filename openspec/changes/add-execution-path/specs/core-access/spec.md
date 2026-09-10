## MODIFIED Requirements

### Requirement: Rights are the CRUD letters
The system SHALL encode rights as CRUD, as the paper fixes them: **C** contribute claims to
the branch, **R** read, **U** update by overlaying a claim with a newer version, and **D**
delete. There SHALL be no fifth right: what an **A** for admin once named is **C** on the
reserved `$branches`, the branch table being itself a claim that such an act contributes to.

Creating a branch SHALL require **C** on `$branches`, which is one server-wide surface
carrying no glob, so the grant admits any name. Contributing claims into a branch SHALL
require **C** on that branch. The two are distinct: the paper's own example, `webapp CR
foo_*, provisioner C $branches`, gives the creating account no grant over `foo_*` at all
(§Access Control).

Deleting a claim held in several branches SHALL require **D** on every branch that holds it,
since a purge removes bytes the others share.

#### Scenario: Creating and writing are separate grants
- **WHEN** `provisioner C $branches` creates `foo_bar` and `webapp CR foo_*` contributes claims to it
- **THEN** both acts are permitted, neither grant conferring the other

#### Scenario: A contribution that would create a branch needs the table right
- **WHEN** an account holding only `CR foo_*` contributes to `foo_bar`, which the base does not carry
- **THEN** the contribution is refused, creating the branch being a right it does not hold

#### Scenario: Referencing across branches imports the closure
- **WHEN** an account holds `R` on `b1` and `C` on `b2`, and contributes to `b2` a claim referencing one in `b1`
- **THEN** the reference is permitted and the referenced claim's full closure is imported into `b2`

#### Scenario: Deletion needs the right on every holder
- **WHEN** a claim is held in branches `a` and `b` and the account holds `D` on `a` only
- **THEN** the delete is denied
