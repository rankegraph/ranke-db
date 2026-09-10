// package: instance / cmd
// type:    logic
// job:     the running instance every verb addresses — its URL and the one credential it carries
// limits:  transport wiring; the routes are the generated client's (-> openapi/client)
//
// Its own package because the verb packages need it: a subcommand lives in the file its
// name gives it, so `branch create` is branch/create.go, and those cannot reach into main.
package instance

import (
	"context"
	"errors"
	"net/http"

	"github.com/rankegraph/ranke-db/openapi/client"
)

// Instance is the server a verb addresses, as the root flags name it.
type Instance struct {
	URL    string
	Token  string
	APIKey string
}

// Connect builds a client carrying the credential on every request. Both schemes are
// accepted so one binary reaches an instance behind either authenticator; naming both is
// refused rather than picking one.
func (i *Instance) Connect() (*client.ClientWithResponses, error) {
	if i.Token != "" && i.APIKey != "" {
		return nil, errors.New("token and api-key are mutually exclusive")
	}
	return client.NewClientWithResponses(i.URL, client.WithRequestEditorFn(
		func(_ context.Context, req *http.Request) error {
			switch {
			case i.Token != "":
				req.Header.Set("Authorization", "Bearer "+i.Token)
			case i.APIKey != "":
				req.Header.Set("X-API-Key", i.APIKey)
			}
			return nil
		}))
}
