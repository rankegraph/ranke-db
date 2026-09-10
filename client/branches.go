// package: client / transport
// type:    adapter
// job:     the branch table — listing it, and reading one branch's head or the archive's
// limits:  transport only; the table is itself a claim the Sequencer mints (-> ranke-go)
//
// Branches are handles, not resources: every head here is a moving target, advanced by
// the next contribution that lands on it.
package client

import "context"

// Branches lists every branch the table holds, each with its current head. It is
// reachable without knowing a branch name, so a client discovers what it may address
// before using the routes that take one, and the listing is answered from one archive
// snapshot, so the heads are consistent with each other.
func (c *Client) Branches(ctx context.Context) ([]BranchEntry, error) {
	res, err := c.api.ListBranchesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200.Branches, nil
}

// BranchHead reads one branch's current head claim id.
func (c *Client) BranchHead(ctx context.Context, branch string) (string, error) {
	res, err := c.api.GetBranchHeadWithResponse(ctx, branch)
	if err != nil {
		return "", err
	}
	if res.JSON200 == nil {
		return "", refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200.Head, nil
}

// BranchInfo reads one branch's head with its height and when it last moved.
func (c *Client) BranchInfo(ctx context.Context, branch string) (*BranchInfo, error) {
	res, err := c.api.GetBranchInfoWithResponse(ctx, branch)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}

// ArchiveInfo reports the Ranke-Archive as a whole. It is the only route that names the
// branch-table head, which is what a client needs to address the $archive scope in a
// query or a grant.
func (c *Client) ArchiveInfo(ctx context.Context) (*ArchiveInfo, error) {
	res, err := c.api.GetArchiveInfoWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}
