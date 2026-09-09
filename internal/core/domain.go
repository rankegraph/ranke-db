// package: core / orchestration
// type:    domain types
// job:     the server's own domain values — merge outcome, liveness, layers, runs
// limits:  types only; the query language is ranke-go's (-> ranke-go)
//
// These are the typed values a Request carries in and out. What they are NOT is
// the graph's vocabulary: the RQL query AST and its result/report shapes were
// drafted here while ranke-go lacked them, and ranke-go now owns them — a Request
// carries a *ranke.Query and the engine answers with a ranke.ResultStream. What
// remains here is genuinely the server's: the outcome of a merge, liveness, the
// storage layers it composed, and the verification-run model it manages.
package core

import (
	"io"
	"time"
)

// --- Contribute / operate --------------------------------------------------

// Contribution is the outcome of a merge: the new head and the contributed ids.
type Contribution struct {
	Head string   `json:"head"`
	Ids  []string `json:"ids"`
}

// Health is liveness plus the contributor identity the stack signs merges with. The
// tags are the wire's: these values are served as they stand, so the names a client
// reads are fixed here rather than translated on the way out.
type Health struct {
	Status  string `json:"status"`           // "ok" when serving
	Version string `json:"version"`          // the server build answering (e.g. "v1.19.2")
	Signer  string `json:"signer,omitempty"` // signing/contributor identity (e.g. "ed25519:…")
}

// StorageLayer names one storage layer — name and type only, by design.
type StorageLayer struct {
	Name string `json:"name"`
	Type string `json:"type"` // backend kind: mem, fs, sqlite, s3, postgres, neo4j, …
}

// Content streams a blob's bytes; the read side of Contribute's io.Reader body.
type Content = io.ReadCloser

// --- Verification ----------------------------------------------------------

// VerificationConfig parameters a run.
type VerificationConfig struct {
	Closure          string            `json:"closure"`                    // root — a branch name or a head id
	Layer            string            `json:"layer,omitempty"`            // optional single storage layer to check directly
	Depth            VerificationDepth `json:"depth,omitempty"`            //
	ContentThreshold int64             `json:"contentThreshold,omitempty"` // for full-content, max bytes checked per claim; 0 = all
}

// VerificationDepth is how thoroughly a run checks each claim.
type VerificationDepth string

const (
	DepthCompleteness      VerificationDepth = "completeness"       // every referenced claim is present
	DepthRecordCorrectness VerificationDepth = "record-correctness" // ids and signatures check out
	DepthFullContent       VerificationDepth = "full-content"       // content matches its hash too
)

// RunStatus is the lifecycle state of a verification run.
type RunStatus string

const (
	RunRunning  RunStatus = "running"
	RunComplete RunStatus = "complete"
	RunStopped  RunStatus = "stopped" // cancelled; partial findings kept
	RunError    RunStatus = "error"
)

// VerificationReport is a point-in-time record of a run.
type VerificationReport struct {
	ID            string                `json:"id"`
	Config        VerificationConfig    `json:"config"`
	Head          string                `json:"head,omitempty"` // the head the run pinned at start
	Status        RunStatus             `json:"status"`
	StartedAt     time.Time             `json:"startedAt"`
	CompletedAt   time.Time             `json:"completedAt,omitzero"` // zero while running
	ClaimsChecked int64                 `json:"claimsChecked"`
	BytesRead     int64                 `json:"bytesRead"`
	OK            bool                  `json:"ok"` // true when no failures were found
	Failures      []VerificationFailure `json:"failures,omitempty"`
}

// VerificationFailure is one integrity problem a run found.
type VerificationFailure struct {
	ID     string      `json:"id"`
	Mode   FailureMode `json:"mode"`
	Layer  string      `json:"layer,omitempty"` // where it was found
	Detail string      `json:"detail,omitempty"`
}

// FailureMode classifies an integrity problem.
type FailureMode string

const (
	FailureCorruptBytes   FailureMode = "corrupt-bytes"   // stored bytes do not match their hash
	FailureInvalidContent FailureMode = "invalid-content" // the claim itself does not validate
)
