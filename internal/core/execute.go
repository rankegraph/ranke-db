// package: core / orchestration
// type:    logic
// job:     turn an Operation into the library call that answers it, and its result into a Stream
// limits:  a seam, no graph behaviour; one archive snapshot per request (-> ranke-go)
//
// Every arm is one call on the archive; an arm that grows logic is in the wrong repository.
//
// A request resolves its snapshot once. RA_k is immutable, so answering from the one
// opened at the start is what keeps a streaming query consistent under a merge.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	ranke "github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/internal/core/access"
	"github.com/rankegraph/ranke-db/internal/version"
)

// execute runs the operation against the ports and returns its response stream.
func (c *Core) execute(ctx context.Context, req *Request) (Stream, error) {
	switch req.Op {
	case OpHealthGet:
		// Health answers from the signer alone, never the archive: it is wanted
		// precisely when the stack is too broken to open one.
		return c.health(ctx, req)
	case OpLayerList, OpLayerInfo:
		return c.layers(req)
	case OpDevClockAdvance:
		return c.devClockAdvanceOp(req)
	}

	archive, err := c.archive(ctx)
	if err != nil {
		return nil, err
	}
	req.Report.step("archive snapshot at head %s", archive.Head())

	switch req.Op {
	case OpClaimContribute:
		return c.contribute(ctx, req)
	case OpClaimQuery:
		return c.query(ctx, req, archive)
	case OpClaimGet:
		return c.claim(ctx, req, archive)
	case OpClaimContent:
		return c.claimContent(ctx, req, archive)
	case OpBranchHead:
		return c.branchHead(ctx, req, archive)
	case OpBranchInfo:
		return c.branchInfo(ctx, req, archive)
	case OpArchiveInfo:
		return c.archiveInfo(ctx, req, archive)
	case OpBranchList:
		return c.branchList(ctx, req, archive)
	case OpVerificationStart, OpVerificationList, OpVerificationGet,
		OpVerificationCancel, OpVerificationDelete:
		return c.verification(ctx, req, archive)
	default:
		req.Report.step("execute %v: not yet implemented", req.Op)
		return nil, ErrNotImplemented
	}
}

// archive resolves the immutable snapshot this request reads from.
func (c *Core) archive(ctx context.Context) (ranke.Archive, error) {
	if c.seq == nil {
		return nil, fmt.Errorf("%w: no sequencer, so no archive to read", ErrNotImplemented)
	}
	archive, err := c.seq.GetArchive(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	return archive, nil
}

// --- the read arms --------------------------------------------------------

// query runs the RQL read and serves its result set.
func (c *Core) query(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	if req.Query == nil {
		return nil, fmt.Errorf("%w: no query", ErrNotFound)
	}
	// Ask for an explicit encoding: the engine's native form is Go objects.
	q := *req.Query
	if q.Output.Encoding == "" || q.Output.Encoding == ranke.ResultNative {
		q.Output.Encoding = ranke.ResultJSON
	}
	results, err := archive.Query(ctx, q)
	if err != nil {
		return nil, mapLibError(err)
	}
	req.Report.step("query executed")
	return newQueryStream(results, q.Output.Encoding), nil
}

// claim serves one claim as the stored CBOR its id signs.
func (c *Core) claim(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	claim, err := c.scopedClaim(ctx, req, archive)
	if err != nil {
		return nil, err
	}
	payload, err := claim.Envelope()
	if err != nil {
		return nil, mapLibError(err)
	}
	req.Report.step("claim %s read", req.ClaimID)
	return &bytesStream{payload: payload, contentType: mediaCBOR}, nil
}

// claimContent streams one claim's content, which inherits that claim's scope.
func (c *Core) claimContent(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	if req.ClaimID == nil {
		return nil, fmt.Errorf("%w: no claim id", ErrNotFound)
	}
	var (
		content io.Reader
		err     error
	)
	switch req.Branch {
	case Universe:
		// No closure to check, so the bytes come off the claim the Universe holds.
		var claim ranke.Claim
		if claim, err = ranke.GetClaim(ctx, c.store, req.ClaimID); err == nil {
			content, err = claim.GetContent(ctx, c.store)
		}
	case Archive:
		content, err = archive.GetClaimContent(ctx, req.ClaimID)
	default:
		var branch ranke.Branch
		if branch, err = c.branch(ctx, req, archive); err == nil {
			content, err = branch.GetClaimContent(ctx, req.ClaimID)
		}
	}
	if err != nil {
		return nil, mapLibError(err)
	}
	req.Report.step("content of %s read", req.ClaimID)
	return &blobStream{content: readCloser(content)}, nil
}

// branchHead serves a branch's current head id.
func (c *Core) branchHead(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	branch, err := c.branch(ctx, req, archive)
	if err != nil {
		return nil, err
	}
	req.Report.step("branch %q head read", req.Branch)
	return &jsonStream{value: branchHead{Head: branch.Head().String()}}, nil
}

// branchInfo serves what is known about one branch: its head, and the head claim's height
// and created_at. Those two are typed fields of the claim the branch points at, read
// through the library's accessors — the branch itself carries neither.
func (c *Core) branchInfo(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	branch, err := c.branch(ctx, req, archive)
	if err != nil {
		return nil, err
	}
	head, err := archive.GetClaim(ctx, branch.Head())
	if err != nil {
		return nil, mapLibError(err)
	}
	req.Report.step("branch %q described", req.Branch)
	return &jsonStream{value: branchInfo{
		Name:      branch.Name(),
		Head:      branch.Head().String(),
		Height:    head.Node().Height(),
		UpdatedAt: head.Node().CreatedAt().UTC(),
	}}, nil
}

// archiveInfo serves the branch-table head and its shape. This is the only place the
// archive head is reported, and it is what a client needs to name the $archive scope.
func (c *Core) archiveInfo(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	head, err := archive.GetClaim(ctx, archive.Head())
	if err != nil {
		return nil, mapLibError(err)
	}
	branches, err := archive.GetBranches(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	req.Report.step("archive described at head %s", archive.Head())
	return &jsonStream{value: archiveInfo{
		Head:      archive.Head().String(),
		Height:    head.Node().Height(),
		UpdatedAt: head.Node().CreatedAt().UTC(),
		Branches:  len(branches),
	}}, nil
}

// branchList serves the branch table's branches by name and head.
func (c *Core) branchList(ctx context.Context, req *Request, archive ranke.Archive) (Stream, error) {
	branches, err := archive.GetBranches(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	named := make([]branchEntry, 0, len(branches))
	for _, b := range branches {
		named = append(named, branchEntry{Name: b.Name(), Head: b.Head().String()})
	}
	req.Report.step("%d branches listed", len(named))
	return &jsonStream{value: branchList{Branches: named}}, nil
}

// scopedClaim reads the request's claim within the scope the request names. The three
// differ in what they reach: a branch is confined to that branch's closure, $archive to
// the closure of the current head across every branch, and $universe to anything the
// Universe holds — which is the privileged read, the one that can reach an archive from
// a Universe and a head id alone, so it must not go through a closure at all.
func (c *Core) scopedClaim(ctx context.Context, req *Request, archive ranke.Archive) (ranke.Claim, error) {
	if req.ClaimID == nil {
		return nil, fmt.Errorf("%w: no claim id", ErrNotFound)
	}
	switch req.Branch {
	case Universe:
		claim, err := ranke.GetClaim(ctx, c.store, req.ClaimID)
		return claim, mapLibError(err)
	case Archive:
		claim, err := archive.GetClaim(ctx, req.ClaimID)
		return claim, mapLibError(err)
	}
	branch, err := c.branch(ctx, req, archive)
	if err != nil {
		return nil, err
	}
	claim, err := branch.GetClaim(ctx, req.ClaimID)
	return claim, mapLibError(err)
}

// branch resolves the request's branch. GetBranch says so itself when the table holds no
// such name, so the error in hand classifies the absence: no second read, and no window in
// which the branch table moves between the two. ErrBranchNotFound rather than the broader
// ErrNotFound it also matches, so a missing branch is answered as one.
func (c *Core) branch(ctx context.Context, req *Request, archive ranke.Archive) (ranke.Branch, error) {
	branch, err := archive.GetBranch(ctx, req.Branch)
	if err == nil {
		return branch, nil
	}
	if errors.Is(err, ranke.ErrBranchNotFound) {
		return nil, fmt.Errorf("%w: branch %q", ErrNotFound, req.Branch)
	}
	return nil, mapLibError(err)
}

// --- operational arms -----------------------------------------------------

// health reports liveness and the signing identity, taken from the signer so it answers
// when no archive opened.
func (c *Core) health(ctx context.Context, req *Request) (Stream, error) {
	report := Health{Status: "ok", Version: version.String()}
	if c.signer != nil {
		report.Signer = signer.Identity(ctx, c.signer)
	}
	req.Report.step("health reported")
	return &jsonStream{value: report}, nil
}

// layers reports what config retained of each layer: a name and a type.
func (c *Core) layers(req *Request) (Stream, error) {
	req.Report.step("%d storage layers listed", len(c.layerInfo))
	return &jsonStream{value: layerList{Layers: c.layerInfo}}, nil
}

// devClockAdvanceOp steers the launch's dev clock, refusing when none was wired —
// no --dev, or a sequencer.type other than "dev" (config.build's own guard).
func (c *Core) devClockAdvanceOp(req *Request) (Stream, error) {
	if c.devClockAdvance == nil {
		return nil, fmt.Errorf("%w: not launched --dev against a dev sequencer", ErrNotImplemented)
	}
	at := c.devClockAdvance(req.DevClockAt)
	req.Report.step("dev clock advanced to %s", at)
	return &jsonStream{value: devClock{Time: at}}, nil
}

// --- wire values ----------------------------------------------------------

// branchHead is one branch's current head.
type branchHead struct {
	Head string `json:"head"`
}

// branchInfo is what is known about one branch.
type branchInfo struct {
	Name      string    `json:"name"`
	Head      string    `json:"head"`
	Height    uint64    `json:"height"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// archiveInfo is what is known about the archive as a whole.
type archiveInfo struct {
	Head      string    `json:"head"`
	Height    uint64    `json:"height"`
	UpdatedAt time.Time `json:"updatedAt"`
	Branches  int       `json:"branches"`
}

// branchEntry names one branch and the head it points at.
type branchEntry struct {
	Name string `json:"name"`
	Head string `json:"head"`
}

// branchList is the branch table's branches.
type branchList struct {
	Branches []branchEntry `json:"branches"`
}

// devClock is the dev clock's position after an advance request.
type devClock struct {
	Time time.Time `json:"time"`
}

// layerList is the storage stack's layers, top to bottom.
type layerList struct {
	Layers []StorageLayer `json:"layers"`
}

// --- error mapping --------------------------------------------------------

// mapLibError resolves a library failure onto its sentinel. Absent and out-of-closure
// arrive as one ErrNotFound, which is what keeps them indistinguishable.
func mapLibError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ranke.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	case errors.Is(err, ranke.ErrUnsupported):
		return fmt.Errorf("%w: %v", ErrNotImplemented, err)
	case errors.Is(err, ranke.ErrQueryTimeOperand):
		// A time field takes one spelling (`R-QTIMEOP`): a `V-TIME` timestamp, or an
		// EDTF Level 1 value on `dated`. A loosely written bound used to be compared as
		// text against the stored fixed-width form and named the wrong instant, so this
		// is the caller's query to correct.
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	case errors.Is(err, ranke.ErrSequencerGenesis):
		// The server stands, holding no archive to answer from. A launch refuses this
		// state outright, so reaching it means founding happened elsewhere — retry once
		// it has.
		return fmt.Errorf("%w: %v", ErrBusy, err)
	case errors.Is(err, ranke.ErrBranchNotCreatable),
		errors.Is(err, ranke.ErrUnreadableReference),
		errors.Is(err, ranke.ErrReservedType):
		// A constraint refusal: the contribution asked for more than its grants gave, so
		// this is the caller's answer to receive, not an internal fault.
		return fmt.Errorf("%w: %v", ErrForbidden, err)
	default:
		return err
	}
}

// readCloser adapts a reader the library handed back, which may or may not own
// resources, to the Close the stream contract requires.
func readCloser(r io.Reader) io.ReadCloser {
	if rc, ok := r.(io.ReadCloser); ok {
		return rc
	}
	return io.NopCloser(r)
}

// --- the contribute arm ---------------------------------------------------

// contribute narrows what a body asked for to what its grants allow, then opens under that
// and merges. A declaration only ever restricts, so the overlap is what the Sequencer
// enforces — a client cannot widen its reach, and the Sequencer never learns whose it is.
func (c *Core) contribute(ctx context.Context, req *Request) (Stream, error) {
	if req.Body == nil {
		return nil, fmt.Errorf("%w: no contribution body", ErrInvalidRequest)
	}

	wire := ranke.NewWireReader(req.Body)
	asked, err := wire.Constraints()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	if len(asked.Branches) == 0 {
		return nil, fmt.Errorf("%w: the contribution declares no branches", ErrInvalidRequest)
	}
	archive, err := c.archive(ctx)
	if err != nil {
		return nil, err
	}
	for _, branch := range asked.Branches {
		if err := c.allow(req, access.Contribute, branch); err != nil {
			return nil, err
		}
	}
	granted, err := c.readableScopes(ctx, req, archive)
	if err != nil {
		return nil, err
	}
	creatable, err := c.creatableBranches(ctx, req, archive, asked.Branches)
	if err != nil {
		return nil, err
	}
	// Branches pass through, each having just been checked for C.
	effective := asked.Narrow(ranke.WireConstraints{
		Branches:     asked.Branches,
		Referencable: granted,
		Lifted:       c.liftedTypes(req),
		Creatable:    creatable,
	})
	req.Report.step("may reference %v, may create %v", effective.Referencable, effective.Creatable)

	contribution, err := c.seq.NewContribution(ctx, effective.Options()...)
	if err != nil {
		return nil, mapLibError(err)
	}
	if err := contribution.AddWire(ctx, wire); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	verified, err := contribution.CompleteAndVerify(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	mergable, err := verified.Persist(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	receipt, err := c.seq.Merge(ctx, mergable)
	if err != nil {
		return nil, mapLibError(err)
	}

	ids := make([]string, 0, len(verified.Ids()))
	for _, id := range verified.Ids() {
		ids = append(ids, id.String())
	}
	req.Report.step("merged %d claim(s) onto %v at head %s", len(ids), asked.Branches, receipt.Head())
	return &jsonStream{value: Contribution{Head: receipt.Head().String(), Ids: ids}}, nil
}

// readableScopes are the scopes this principal may reference claims from, named as the library
// names them — which is what turns a grant over globs into a set the Sequencer can enforce.
func (c *Core) readableScopes(ctx context.Context, req *Request, archive ranke.Archive) ([]string, error) {
	if c.access.Allow(req.Principal, access.Read, Universe) {
		return []string{ranke.BranchUniverse}, nil
	}
	branches, err := archive.GetBranches(ctx)
	if err != nil {
		return nil, mapLibError(err)
	}
	scopes := make([]string, 0, len(branches))
	for _, b := range branches {
		if c.access.Allow(req.Principal, access.Read, b.Name()) {
			scopes = append(scopes, b.Name())
		}
	}
	return scopes, nil
}

// liftedTypes are the reserved node types this principal may carry. C on $branches — what an
// A for admin once named — lifts the branch-table type; the limiting claims stay reserved.
func (c *Core) liftedTypes(req *Request) []string {
	if c.access.Allow(req.Principal, access.Contribute, Branches) {
		return []string{ranke.NodeBranches}
	}
	return nil
}

// creatableBranches are the declared branches the base lacks, which this contribution would
// bring into being. Creating one is its own right, C over the branch table.
func (c *Core) creatableBranches(ctx context.Context, req *Request, archive ranke.Archive, declared []string) ([]string, error) {
	missing, err := archive.MissingBranches(ctx, declared)
	if err != nil {
		return nil, mapLibError(err)
	}
	if len(missing) == 0 {
		return nil, nil
	}
	if !c.access.Allow(req.Principal, access.Contribute, Branches) {
		return nil, nil
	}
	req.Report.step("%d branch(es) would be created: %v", len(missing), missing)
	return missing, nil
}
