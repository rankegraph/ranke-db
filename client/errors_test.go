// package: client / transport
// type:    test
// job:     a refusal onto its sentinel — the categories the real endpoint emits, end to end, and
// the rest against the same {code, error} body the server writes
// limits:  the mapping; which category a route emits is the server's
package client_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/auth/apikey/apikeytest"
	"github.com/rankegraph/ranke-db/client"
	"github.com/rankegraph/ranke-db/internal/core"
	"github.com/rankegraph/ranke-db/openapi"
)

// TestRefusalsFromTheRealEndpoint pins the categories a running stack produces, each
// through the route that produces it.
func TestRefusalsFromTheRealEndpoint(t *testing.T) {
	ctx := context.Background()

	t.Run("unauthenticated", func(t *testing.T) {
		cfg, _, done := apikeytest.Setup(t, testAccount)
		t.Cleanup(done)
		_, c := serve(t, withAuth(cfg), as(client.WithAPIKey("not-the-configured-key")))
		_, err := c.Whoami(ctx)
		assertCategory(t, err, client.ErrUnauthenticated, core.CatUnauthenticated)
	})

	t.Run("forbidden", func(t *testing.T) {
		// Every right but the one the archive scope is read under.
		_, c := serve(t, withGrants("CR *", "R $branches"))
		_, err := c.ArchiveInfo(ctx)
		assertCategory(t, err, client.ErrForbidden, core.CatForbidden)
	})

	t.Run("not_found", func(t *testing.T) {
		_, c := serve(t)
		_, err := c.BranchInfo(ctx, "no-such-branch")
		assertCategory(t, err, client.ErrNotFound, core.CatNotFound)
	})

	t.Run("invalid", func(t *testing.T) {
		s, c := serve(t)
		// The note references a contributor claim that neither the archive holds nor the
		// stream carries, so the closure cannot resolve the signature over it.
		_, err := c.Contribute(ctx, s.Universe, testBranch, []ranke.Claim{s.note(t, "orphaned", storyTime)})
		assertCategory(t, err, client.ErrInvalid, core.CatInvalid)
	})
}

// TestRefusalMapsEveryCategory pins the whole table, including the two a stack of this
// size will not produce on demand. The bodies are the server's own — core's Category
// constants rendered through the contract's Error model, which is what writeError puts
// on the wire — so renaming a category breaks this rather than leaving it green.
func TestRefusalMapsEveryCategory(t *testing.T) {
	for _, tc := range []struct {
		cat    core.Category
		status int
		want   error
	}{
		{core.CatUnauthenticated, http.StatusUnauthorized, client.ErrUnauthenticated},
		{core.CatForbidden, http.StatusForbidden, client.ErrForbidden},
		{core.CatNotFound, http.StatusNotFound, client.ErrNotFound},
		{core.CatConflict, http.StatusConflict, client.ErrConflict},
		{core.CatBusy, http.StatusTooManyRequests, client.ErrBusy},
		{core.CatInvalid, http.StatusBadRequest, client.ErrInvalid},
		{core.CatUnimplemented, http.StatusNotImplemented, client.ErrUnimplemented},
	} {
		t.Run(string(tc.cat), func(t *testing.T) {
			c := refusing(t, tc.status, openapi.Error{Code: string(tc.cat), Error: "refused"})
			_, err := c.Health(context.Background())
			if !errors.Is(err, tc.want) {
				t.Fatalf("Health = %v, want %v", err, tc.want)
			}
			var refused *client.Error
			if !errors.As(err, &refused) {
				t.Fatalf("Health = %v, want an *Error carrying the category", err)
			}
			if refused.Code != string(tc.cat) || refused.Status != tc.status {
				t.Fatalf("Error = %+v, want code %q at %d", refused, tc.cat, tc.status)
			}
		})
	}
}

// TestRefusalWithoutTheContractsBody pins the answer from something that is not this
// server — a proxy, a load balancer — where the status is all there is to read.
func TestRefusalWithoutTheContractsBody(t *testing.T) {
	c := refusing(t, http.StatusNotFound, "<html>404 not found</html>")
	_, err := c.Health(context.Background())
	if !errors.Is(err, client.ErrNotFound) {
		t.Fatalf("Health = %v, want ErrNotFound from the status alone", err)
	}
	var refused *client.Error
	if !errors.As(err, &refused) || refused.Code != "" {
		t.Fatalf("Error = %v, want no code where the body carried none", err)
	}
}

// TestCategoryOutsideTheTable pins a code this client does not name: it arrives as an
// *Error carrying it, rather than matching a sentinel it is not. The status is a 429,
// which is the sentinel it would land on were the status read behind a code that came.
func TestCategoryOutsideTheTable(t *testing.T) {
	c := refusing(t, http.StatusTooManyRequests, openapi.Error{Code: "quota_exhausted", Error: "pay up"})
	_, err := c.Health(context.Background())
	for _, sentinel := range []error{
		client.ErrUnauthenticated, client.ErrForbidden, client.ErrNotFound,
		client.ErrConflict, client.ErrBusy, client.ErrInvalid, client.ErrUnimplemented,
	} {
		if errors.Is(err, sentinel) {
			t.Fatalf("an unknown code matched %v", sentinel)
		}
	}
	var refused *client.Error
	if !errors.As(err, &refused) || refused.Code != "quota_exhausted" {
		t.Fatalf("Health = %v, want the code carried through", err)
	}
}

// refusing answers every request with one status and body, for the categories a stack
// of this size does not produce to order.
func refusing(t *testing.T, status int, body any) *client.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch v := body.(type) {
		case string:
			_, _ = w.Write([]byte(v))
		default:
			_ = json.NewEncoder(w).Encode(v)
		}
	}))
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	return c
}

// assertCategory checks an error against the sentinel it should match and the
// category the server names it by.
func assertCategory(t *testing.T, err error, sentinel error, cat core.Category) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	var refused *client.Error
	if !errors.As(err, &refused) || refused.Code != string(cat) {
		t.Fatalf("err = %v, want the code %q", err, cat)
	}
}
