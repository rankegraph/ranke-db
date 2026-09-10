// package: client / transport
// type:    domain types
// job:     the contract's JSON payloads, under this package's name
// limits:  aliases only; every shape is generated from openapi/openapi.yaml, so a spec change
// moves the field here too rather than leaving a parallel copy behind
package client

import api "github.com/rankegraph/ranke-db/openapi/client"

// The models a route answers with. They are aliases rather than copies: a second
// declaration of the same payload is a second source of truth, and the one thing an
// OpenAPI contract exists to prevent.
type (
	// ArchiveInfo reports the Ranke-Archive as a whole — the branch-table head every
	// $archive-scoped read is rooted at, with its height and when it last moved.
	ArchiveInfo = api.ArchiveInfo
	// BranchEntry is one row of the branch table: a name and the head it resolves to.
	BranchEntry = api.BranchEntry
	// BranchInfo is one branch's head with its height and when it last moved.
	BranchInfo = api.BranchInfo
	// ContributionResult is what a merge produced: the new branch-table head and the
	// ids absorbed under it.
	ContributionResult = api.ContributionResult
	// Health is the stack's liveness, the identity it merges under, and its build.
	Health = api.Health
	// StorageLayer names one read-through tier and the adapter behind it.
	StorageLayer = api.StorageLayer
	// Subject is what a credential resolved to: the account, its grants, and the
	// caveats the credential itself carries.
	Subject = api.Subject
	// VerificationConfig is a run's parameters — the closure, the depth, the layer.
	VerificationConfig = api.VerificationConfig
	// VerificationFailure is one claim or object a run found wanting.
	VerificationFailure = api.VerificationFailure
	// VerificationReport is a point-in-time record of a run, config included.
	VerificationReport = api.VerificationReport
)
