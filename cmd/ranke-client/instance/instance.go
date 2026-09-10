// package: instance / cmd
// type:    logic
// job:     the running instance every verb addresses — its URL and the one credential it carries
// limits:  wiring only; the requests are the official client's (-> client)
//
// Its own package because the verb packages need it: a subcommand lives in the file its
// name gives it, so `branch create` is branch/create.go, and those cannot reach into main.
package instance

import "github.com/rankegraph/ranke-db/client"

// Instance is the server a verb addresses, as the root flags name it.
type Instance struct {
	URL      string
	Token    string
	APIKey   string
	Macaroon string
}

// Connect builds a client carrying the credential on every request. All three schemes
// are accepted so one binary reaches an instance behind any authenticator; naming more
// than one is refused rather than picking one, since the endpoint routes on the scheme
// presented and two of them name two adapters.
func (i *Instance) Connect() (*client.Client, error) {
	var opts []client.Option
	if i.Token != "" {
		opts = append(opts, client.WithToken(i.Token))
	}
	if i.APIKey != "" {
		opts = append(opts, client.WithAPIKey(i.APIKey))
	}
	if i.Macaroon != "" {
		opts = append(opts, client.WithMacaroon(i.Macaroon))
	}
	return client.New(i.URL, opts...)
}
