// package: client / transport
// type:    errors
// job:     a refused request as a Go error — the contract's {code, error} body behind the seven
// categories core emits, each an errors.Is sentinel
// limits:  naming failures only; who emits which is core's (-> internal/core/core.go)
package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// The categories the server emits, as sentinels to branch on. A code the server adds
// later arrives as an *Error with that Code and no sentinel, rather than as a match on
// the wrong one.
var (
	// ErrUnauthenticated is a missing or invalid credential.
	ErrUnauthenticated = errors.New("ranke/client: unauthenticated")
	// ErrForbidden is an authenticated account without the grant the request needs.
	ErrForbidden = errors.New("ranke/client: forbidden")
	// ErrNotFound is an unknown or out-of-scope branch, claim or content. For a by-id
	// read the two are indistinguishable, which is the point of a scoped closure.
	ErrNotFound = errors.New("ranke/client: not found")
	// ErrConflict is a contribution that clashes with the head it would merge onto.
	ErrConflict = errors.New("ranke/client: conflict")
	// ErrBusy is a refusal for want of a free slot, which is worth retrying.
	ErrBusy = errors.New("ranke/client: busy")
	// ErrInvalid is a malformed request — including both credentials at once, which the
	// endpoint routes on the presented scheme and so cannot resolve.
	ErrInvalid = errors.New("ranke/client: invalid request")
	// ErrUnimplemented is a capability this stack does not offer, which is how a
	// production stack answers the dev-only routes.
	ErrUnimplemented = errors.New("ranke/client: not implemented")
)

// byCode maps the contract's stable category onto its sentinel.
var byCode = map[string]error{
	"unauthenticated": ErrUnauthenticated,
	"forbidden":       ErrForbidden,
	"not_found":       ErrNotFound,
	"conflict":        ErrConflict,
	"busy":            ErrBusy,
	"invalid":         ErrInvalid,
	"unimplemented":   ErrUnimplemented,
}

// byStatus maps a status onto the same sentinel, for an answer that carried no code —
// a proxy's 404, a load balancer's 503.
var byStatus = map[int]error{
	http.StatusUnauthorized:    ErrUnauthenticated,
	http.StatusForbidden:       ErrForbidden,
	http.StatusNotFound:        ErrNotFound,
	http.StatusConflict:        ErrConflict,
	http.StatusTooManyRequests: ErrBusy,
	http.StatusBadRequest:      ErrInvalid,
	http.StatusNotImplemented:  ErrUnimplemented,
}

// Error is a request the server refused. Code is the contract's machine-readable
// category and is what to branch on — through errors.Is against the sentinels above,
// so a misspelt code is a compile error rather than a comparison that never matches.
type Error struct {
	Status  int
	Code    string
	Message string
}

// Error names the category, the status and what the server said.
func (e *Error) Error() string {
	name := e.Code
	if name == "" {
		name = strings.ToLower(http.StatusText(e.Status))
	}
	return fmt.Sprintf("ranke/client: %s (HTTP %d): %s", name, e.Status, e.Message)
}

// Unwrap returns the sentinel this category stands for, so errors.Is answers for it.
// A category the client does not name unwraps to nil, leaving the *Error itself: the
// status is read only where no code came with it, since a code the server adds later
// would otherwise match on whichever sentinel its status happens to share.
func (e *Error) Unwrap() error {
	if e.Code == "" {
		return byStatus[e.Status]
	}
	return byCode[e.Code]
}

// maxErrorBody caps what is read from a refusal, which is a short JSON document
// whenever it comes from this contract.
const maxErrorBody = 1 << 16

// refusal builds an *Error from a status and the body that came with it, falling back
// to the status text where the body is not the contract's error shape.
func refusal(status int, body []byte) *Error {
	e := &Error{Status: status, Message: http.StatusText(status)}
	if len(body) == 0 {
		return e
	}
	var wire struct {
		Code  string `json:"code"`
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &wire) == nil && wire.Code != "" {
		e.Code, e.Message = wire.Code, wire.Error
		return e
	}
	if msg := strings.TrimSpace(string(body)); msg != "" {
		e.Message = msg
	}
	return e
}
