package core

import (
	"context"
	"errors"
	"testing"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/config/scope"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// newTestCore assembles a core with a NoAuth backend authenticating as "ops" and
// an access policy granting ops read on foo_*. The driven ports are nil: the
// pipeline reaches the execute stub, which touches neither.
func newTestCore(t *testing.T) *Core {
	t.Helper()
	ctx := context.Background()

	a, err := auth.New(ctx, scope.Literal(map[string]string{"type": "noauth", "subject": "ops"}))
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	set, err := auth.NewSet([]auth.Auth{a})
	if err != nil {
		t.Fatalf("auth.NewSet: %v", err)
	}
	chk, err := access.New(map[string][]string{"ops": {"R foo_*"}})
	if err != nil {
		t.Fatalf("access.New: %v", err)
	}
	return New(set, chk, nil, nil)
}

// TestHandleFlow walks a request through the whole pipeline: an empty-credential
// request falls back to NoAuth, authorizes against the grant, and reaches execution —
// which here has no archive to open, since the test core binds no ports.
func TestHandleFlow(t *testing.T) {
	c := newTestCore(t)
	req := &Request{Op: OpClaimQuery, Branch: "foo_bar"}

	_, err := c.Handle(context.Background(), req)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("Handle err = %v, want ErrNotImplemented (no archive is bound)", err)
	}
	if req.Principal.Account != "ops" {
		t.Fatalf("principal = %q, want ops", req.Principal.Account)
	}
	if len(req.Report.Steps) != 2 {
		t.Fatalf("report steps = %v, want authenticate+authorize", req.Report.Steps)
	}
}

// TestHandleForbidden stops the pipeline at authorization: ops holds R on foo_*
// only, so a read of bar-baz is forbidden and never reaches execution.
func TestHandleForbidden(t *testing.T) {
	c := newTestCore(t)
	req := &Request{Op: OpClaimQuery, Branch: "bar-baz"}

	_, err := c.Handle(context.Background(), req)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("Handle err = %v, want ErrForbidden", err)
	}
	if len(req.Report.Steps) != 1 {
		t.Fatalf("report steps = %v, want only the authenticate step", req.Report.Steps)
	}
}
