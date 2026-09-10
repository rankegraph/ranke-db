// package: client / transport
// type:    adapter
// job:     the verification runs — start one, poll it, cancel it, delete it
// limits:  transport only; the three depths and what each proves are the library's
// (-> ranke-go verify)
package client

import (
	"context"
	"net/http"
)

// Verifications lists the runs the stack holds, running and finished alike.
func (c *Client) Verifications(ctx context.Context) ([]VerificationReport, error) {
	res, err := c.api.ListVerificationsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200.Reports, nil
}

// StartVerification begins a run over cfg's closure and returns the report it opens
// with. The run is asynchronous: poll Verification until status leaves running.
//
// A stack already at its run limit refuses with ErrBusy, which is the one refusal here
// worth retrying.
func (c *Client) StartVerification(ctx context.Context, cfg VerificationConfig) (*VerificationReport, error) {
	res, err := c.api.StartVerificationWithResponse(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if res.JSON202 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON202, nil
}

// Verification reads one run's report. While the run is still going the progress
// counters advance between polls.
func (c *Client) Verification(ctx context.Context, id string) (*VerificationReport, error) {
	res, err := c.api.GetVerificationWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}

// CancelVerification stops a running run and keeps its report, partial findings and
// all, freeing the concurrency slot. It is idempotent: cancelling a run that already
// finished returns its report unchanged.
func (c *Client) CancelVerification(ctx context.Context, id string) (*VerificationReport, error) {
	res, err := c.api.CancelVerificationWithResponse(ctx, id)
	if err != nil {
		return nil, err
	}
	if res.JSON200 == nil {
		return nil, refusal(res.StatusCode(), res.Body)
	}
	return res.JSON200, nil
}

// DeleteVerification stops the run if it is still going, then removes the record — so
// a subsequent read is a not-found. Cancel keeps the report; this does not.
func (c *Client) DeleteVerification(ctx context.Context, id string) error {
	res, err := c.api.DeleteVerificationWithResponse(ctx, id)
	if err != nil {
		return err
	}
	if res.StatusCode() != http.StatusNoContent {
		return refusal(res.StatusCode(), res.Body)
	}
	return nil
}
