// package: core / orchestration
// type:    struct
// job:     the one request object that flows through the server, enriched at each stage
// limits:  transport-neutral; endpoints fill the ingress, core the rest (-> core.go)
//
// Package core is the hexagon's center: it drives the driven ports (storage,
// sequencer, signer) and is driven by the driving ports (auth, endpoints). Its
// spine is one Request value that flows endpoint → auth → access → execution; the
// response comes back as a Stream (see stream.go, core.go). The endpoint fills the
// ingress fields from the wire; core fills the enrichment.
package core

import (
	"fmt"
	"io"
	"time"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// The reserved pseudo-branches, re-exported from access so a request targets them
// through core alone. They are the scopes a read names: Universe the privileged read
// under no closure, Archive the closure of the whole archive across every branch, and
// Branches the branch table itself.
const (
	Universe = access.Universe
	Archive  = access.Archive
	Branches = access.Branches
)

// Operation is what a request asks for, named Subject+Verb so it sorts hierarchically.
// It fixes the right the authorize stage checks (0 = none) and selects the execute
// branch; the pseudo-branches generalise scope, so there is no separate universe op.
type Operation int

const (
	OpClaimQuery         Operation = iota // query claims                           (Read)
	OpClaimGet                            // one claim by id (branch or $universe)   (Read)
	OpClaimContent                        // one claim's content                    (Read)
	OpClaimContribute                     // merge claims onto a branch             (Contribute; branch-admin is C on the branch table)
	OpClaimDelete                         // purge claims — physical removal, not a mutation (Delete)
	OpBranchHead                          // a branch's current head id             (Read)
	OpBranchInfo                          // a branch's head, height and last move  (Read)
	OpBranchList                          // list the branch table's branches       (Read on $branches)
	OpArchiveInfo                         // the branch-table head and its shape    (Read on $archive)
	OpLayerList                           // list storage layers (name + type)      (no grant)
	OpLayerInfo                           // runtime info on one storage layer      (no grant)
	OpHealthGet                           // liveness                               (no grant)
	OpSubjectGet                          // the caller's own account and grants    (no grant)
	OpVerificationStart                   // start a verification run               (no grant — verification needs none)
	OpVerificationList                    // list verification runs                 (no grant)
	OpVerificationGet                     // one verification run                   (no grant)
	OpVerificationCancel                  // cancel a run                           (no grant)
	OpVerificationDelete                  // delete a run                           (no grant)
	OpDevClockAdvance                     // steer the --dev clock forward          (no grant)
)

// Right is the right this operation requires, or 0 for none. Reads need R, writes
// their CRUD letter; verification needs none, a core-access invariant.
func (o Operation) Right() access.Right {
	switch o {
	case OpClaimQuery, OpClaimGet, OpClaimContent, OpBranchHead, OpBranchInfo, OpBranchList, OpArchiveInfo:
		return access.Read
	case OpClaimContribute:
		return access.Contribute
	case OpClaimDelete:
		return access.Delete
	}
	return 0
}

// Request is the single object that flows through the server, enriched at each stage.
// Ingress fields are op-specific; the response is not a field — Handle returns a Stream.
type Request struct {
	// --- ingress: filled by the endpoint from the wire ---

	// Credential is the one token the transport extracted, or the zero value (→ NoAuth);
	// more than one scheme is rejected by the endpoint before this is built.
	Credential auth.Credential
	// Op is what the caller wants; it fixes the required access right.
	Op Operation
	// Branch is the target branch, or Universe for a privileged by-id read.
	Branch string
	// Query is the read AST (OpClaimQuery) — ranke-go's RQL, answered by the Universe.
	Query *ranke.Query
	// Body is the signed-CBOR claims to merge (OpClaimContribute).
	Body io.Reader
	// ClaimID targets one claim (OpClaimGet, OpClaimContent). Content is addressed by
	// its claim, inheriting that scope and hiding whether the bytes are inline.
	ClaimID ranke.Id
	// VerConfig parameters a run (OpVerificationStart).
	VerConfig *VerificationConfig
	// VerID targets one run (OpVerificationGet, OpVerificationCancel, OpVerificationDelete).
	VerID string
	// DevClockAt is the instant to advance the dev clock to (OpDevClockAdvance).
	DevClockAt time.Time

	// --- enrichment: filled by core as the request flows ---

	// Principal is who the request authenticated as (set by the authenticate stage).
	Principal access.Principal
	// Report is the running execution trace, accumulated across stages.
	Report Report
}

// Report is the execution record that travels with the request: a human-readable
// trace now, and the home for the witnessed transaction window and per-stage
// counts as those stages are built.
type Report struct {
	Steps []string
}

// step appends one line to the running trace.
func (r *Report) step(format string, args ...any) {
	r.Steps = append(r.Steps, fmt.Sprintf(format, args...))
}
