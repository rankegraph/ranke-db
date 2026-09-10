// package: contributor / cmd
// type:    logic
// job:     resolve a --signing-key argument to the contributor identity it names
// limits:  a seam over keysource and ParseKeypair; the grammar and its refusals are the
// library's (-> github.com/rankegraph/ranke-go/keysource)
//
// A contributor key is application-held: it signs claims into their ids and never reaches
// a server. WithTTY is granted here because this is a tool a person runs — a server must
// not, or it can be stopped on a terminal read.
package contributor

import (
	"io"

	"github.com/rankegraph/ranke-go"
	"github.com/rankegraph/ranke-go/keysource"
)

// Load resolves spec — a path, file:PATH, env:NAME, stdin, or prompt — to the keypair it
// names. in is where "stdin" reads from.
func Load(spec string, in io.Reader) (ranke.Keypair, error) {
	pemBytes, err := keysource.Load(spec, in, keysource.WithTTY())
	if err != nil {
		return ranke.Keypair{}, err
	}
	return ranke.ParseKeypair(pemBytes)
}
