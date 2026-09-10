// package: client / transport
// type:    adapter
// job:     the system routes — what this credential resolves to, and what the stack is made of
// limits:  transport only; the answers are the stack's configuration (-> config)
package client

import "context"

// Whoami reports the account this credential resolves to, the grants it holds and the
// caveats the credential itself carries. It needs no grant, so it answers for an
// account holding nothing — which is the case worth asking about.
func (c *Client) Whoami(ctx context.Context) (*Subject, error) {
	res, err := c.api.WhoamiWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}

// Layers lists the storage stack's read-through tiers, top (cache) to bottom
// (authoritative). A layer's name is what a query pins execution to and what a
// verification run reads directly.
func (c *Client) Layers(ctx context.Context) ([]StorageLayer, error) {
	res, err := c.api.ListStorageLayersWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200.Layers, nil
}
