// package: config / composition
// type:    logic
// job:     founding — turn an empty bookmark list into (𝒰, k₀), from the config or `ranke-db found`
// limits:  reads the founder key and calls Sequencer.Found; the claims are ranke-go's (-> ranke-go)
//
// A launch resolves genesis before it serves: it founds when the config names a founder,
// and refuses when it cannot.
package config

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"

	"github.com/rankegraph/ranke-go"

	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/config/scope"
)

// ErrAlreadyFounded reports an archive that needs no founding. `ranke-db found` answers
// it as success, so provisioning may call it before every launch.
var ErrAlreadyFounded = errors.New("config: the archive is already founded")

// Founding carries the ids an operator must keep from a launch that created an archive.
type Founding struct {
	// FirstContributor is the only trace of a key whose private half the server never held.
	FirstContributor ranke.Id
	// Head is k₀, the empty branch table the archive begins at.
	Head ranke.Id
	// Bookmark is the entry in 𝒰_hist the archive can be reopened from.
	Bookmark ranke.Id
	// Branch is the branch founding bound the first contributor to.
	Branch string
}

// Found assembles the stack from cfg and founds its archive under pubkeyPEM, the first
// contributor's public key, returning as soon as the archive exists. It never serves.
// The branch comes from sequencer.found.branch: which branch an archive begins on is
// permanent, so it belongs in the launch artifact rather than in a caller's argument.
func Found(ctx context.Context, cfg io.Reader, pass PassphraseSource, pubkeyPEM []byte) (*Founding, error) {
	c, err := decode(cfg, pass)
	if err != nil {
		return nil, err
	}
	app, err := c.build(ctx, false)
	if err != nil {
		return nil, err
	}
	if app.Sequencer == nil {
		return nil, errors.New("config: no sequencer is configured, so there is no archive to found")
	}
	if !app.Sequencer.InGenesis() {
		return nil, ErrAlreadyFounded
	}
	branch, err := foundingBranch(ctx, c.section(c.Sequencer))
	if err != nil {
		return nil, err
	}
	return foundWith(ctx, app.Sequencer, pubkeyPEM, branch)
}

// foundIfConfigured resolves the genesis state of a launch. Refusing beats coming up
// without an archive, which would answer every request with ErrSequencerGenesis —
// a fault report where a missing setting is the whole story.
func (c *Config) foundIfConfigured(ctx context.Context, app *App) error {
	if app.Sequencer == nil || !app.Sequencer.InGenesis() {
		return nil
	}
	sec := c.section(c.Sequencer)
	found := sec.GetSection("found")
	if !found.HasValue("pubkey") {
		return errors.New("config: this archive has no first contributor yet: " +
			"set sequencer.found.pubkey to its PEM public key, " +
			"or found it once with \"ranke-db found <config> <pubkey.pem>\"")
	}
	raw, err := found.Get(ctx, "pubkey")
	if err != nil {
		return fmt.Errorf("config: sequencer.found.pubkey: %w", err)
	}
	branch, err := foundingBranch(ctx, sec)
	if err != nil {
		return err
	}
	founding, err := foundWith(ctx, app.Sequencer, []byte(raw), branch)
	if err != nil {
		return err
	}
	app.Founded = founding
	return nil
}

// foundingBranch reads the branch an archive begins on. Founding binds the first
// contributor to it, which is what makes that contributor reachable at all: k₀ takes
// one reference (`V-ARCHIVEHEIGHT`) and cannot name it, so an archive founded without a
// branch is one nobody can write to.
func foundingBranch(ctx context.Context, sec scope.Section) (string, error) {
	found := sec.GetSection("found")
	if !found.HasValue("branch") {
		return "", errors.New("config: sequencer.found.branch is required: " +
			"founding binds the first contributor to a branch, and which one is permanent")
	}
	branch, err := found.Get(ctx, "branch")
	if err != nil {
		return "", fmt.Errorf("config: sequencer.found.branch: %w", err)
	}
	if err := ranke.ValidateBranchName(branch); err != nil {
		return "", fmt.Errorf("config: sequencer.found.branch: %w", err)
	}
	return branch, nil
}

// foundWith founds seq under the PEM public key.
func foundWith(ctx context.Context, seq sequencer.Sequencer, pubkeyPEM []byte, branch string) (*Founding, error) {
	pubkey, err := encodePubkey(pubkeyPEM)
	if err != nil {
		return nil, err
	}
	first, err := seq.Found(ctx, pubkey, branch)
	if err != nil {
		return nil, fmt.Errorf("config: found the archive: %w", err)
	}
	archive, err := seq.GetArchive(ctx)
	if err != nil {
		return nil, fmt.Errorf("config: read the founded archive: %w", err)
	}
	return &Founding{
		FirstContributor: first.ID(),
		Head:             archive.Head(),
		Bookmark:         seq.BookmarkId(),
		Branch:           branch,
	}, nil
}

// encodePubkey converts a PEM public key, the form the jwt authenticator takes one in,
// to the multikey encoding a contributor claim carries as content.
func encodePubkey(pubkeyPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(pubkeyPEM)
	if block == nil {
		return nil, errors.New("config: founder: no PEM block found (want a PUBLIC KEY block)")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("config: founder: parse PEM public key: %w", err)
	}
	encoded, err := ranke.EncodePublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("config: founder: encode public key: %w", err)
	}
	return encoded, nil
}
