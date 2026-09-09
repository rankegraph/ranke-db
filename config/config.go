// package: config / composition
// type:    struct
// job:     decrypt/parse the launch config and either check it (Verify) or assemble the adapter stack (Run)
// limits:  the only component that sees the whole config; adapters get scope.Section slices (-> Verify, Run)
//
// Two entry points mirror the CLI verbs: Verify checks without assembling, Run assembles
// the live stack. Each adapter is handed its own scope.Section and nothing else, its
// env()/vault() delegations resolved lazily at use.
//
// config is the one component that sees the whole config; the narrowing below it is by
// containment, not visibility.
package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/rankegraph/ranke-db/adapters/auth"
	"github.com/rankegraph/ranke-db/adapters/endpoints"
	"github.com/rankegraph/ranke-db/adapters/endpoints/rest_http"
	"github.com/rankegraph/ranke-db/adapters/sequencer"
	"github.com/rankegraph/ranke-db/adapters/signer"
	"github.com/rankegraph/ranke-db/adapters/storage"
	"github.com/rankegraph/ranke-db/internal/core"
	"github.com/rankegraph/ranke-db/internal/core/access"
)

// Config is the parsed launch artifact: driven ports and accounts shared by the
// instance, authentication and admission per endpoint, each section an open map.
type Config struct {
	Signer    section                  `json:"signer"`
	Vault     section                  `json:"vault"`
	Storage   section                  `json:"storage"`
	Sequencer section                  `json:"sequencer"`
	Accounts  map[string]accountConfig `json:"accounts"`
	Endpoints []endpointConfig         `json:"endpoints"`

	// box is the shared secret-store holder, seeded from Vault at parse: the only
	// place a section reaches the vault, built lazily on first vault() use.
	box *vaultBox
}

// accountConfig is one system account: CRUD grants over branch globs, no credential
// material. Grants are pure policy — no env()/vault() delegation — validated at parse.
type accountConfig struct {
	Grants []string `json:"grants"`
}

// endpointConfig is one endpoint: a transport, its auth backends, and the accounts it
// admits. Admission isolates: its access checker is built from the admitted subset.
type endpointConfig struct {
	Transport section   `json:"transport"`
	Auth      []section `json:"auth"`
	Admit     []string  `json:"admit"`
}

// section is one adapter's raw config: undecoded JSON, resolved per value on read.
type section map[string]json.RawMessage

// App is the assembled stack Run produces: the shared driven ports and the endpoints,
// each holding its own core over them.
type App struct {
	Storage   storage.Storage
	Signer    signer.Signer
	Sequencer sequencer.Sequencer
	Endpoints []endpoints.Endpoints
	// Layers names the configured storage layers, name and type only.
	Layers []storage.Layer
	// DevClock is the launch's steerable clock, non-nil only when Run was called with
	// dev true — the handle POST /dev/clock advances (-> core.WithDevClock).
	DevClock *sequencer.SteerableClock
	// Founded records the archive this launch brought into being, non-nil only when it
	// founded one — so the launch log can carry the ids an operator must keep.
	Founded *Founding
	// The secret store is omitted on purpose: it lives in the section box, and nobody
	// downstream holds it.
}

// Level is the depth of a Verify check.
type Level int

const (
	// LevelSyntax parses and shape-checks offline — no environment, vault or assembly.
	LevelSyntax Level = iota
	// LevelResolve also resolves every env()/vault() reference, assembling nothing.
	LevelResolve
	// LevelConnect also assembles the adapters and discards them, so a bad backend
	// surfaces before Run does.
	LevelConnect
)

// Verify checks an (optionally age-encrypted) config to the given level, decrypting
// with pass when the bytes are encrypted. See Level for what each depth reaches.
func Verify(ctx context.Context, cfg io.Reader, pass PassphraseSource, level Level) error {
	c, err := decode(cfg, pass)
	if err != nil {
		return err
	}
	if level < LevelResolve {
		return nil
	}
	if err := c.resolveAll(ctx); err != nil {
		return err
	}
	if level < LevelConnect {
		return nil
	}
	// Assembling the stack reaches every backend it uses; discard it without serving.
	// Never --dev: a check has no business minting a steerable clock.
	_, err = c.build(ctx, false)
	return err
}

// Run decrypts with pass when needed, parses, and assembles the stack. dev mounts the
// launch's steerable clock — see App.DevClock — and requires sequencer.type "dev";
// pass false for every real deployment.
func Run(ctx context.Context, cfg io.Reader, pass PassphraseSource, dev bool) (*App, error) {
	c, err := decode(cfg, pass)
	if err != nil {
		return nil, err
	}
	app, err := c.build(ctx, dev)
	if err != nil {
		return nil, err
	}
	// Founding is Run's, never build's: build is also what Verify(LevelConnect)
	// assembles and discards, and a check has no business creating an archive.
	if err := c.foundIfConfigured(ctx, app); err != nil {
		return nil, err
	}
	return app, nil
}

// decode reads, decrypts (when encrypted), and parses the config — the shared
// front of Verify and Run. pass is consulted only if the bytes are encrypted.
func decode(cfg io.Reader, pass PassphraseSource) (*Config, error) {
	data, err := io.ReadAll(cfg)
	if err != nil {
		return nil, fmt.Errorf("config: read: %w", err)
	}
	plaintext, err := decrypt(data, pass)
	if err != nil {
		return nil, err
	}
	return load(bytes.NewReader(plaintext))
}

// load parses the JSON launch artifact without resolving delegations — the
// offline shape check. Unknown top-level sections are rejected.
func load(r io.Reader) (*Config, error) {
	var c Config
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("config: parse: %w", err)
	}
	// Accounts are pure policy: validate every account's grants now, offline, so a
	// malformed grant fails the syntax check rather than waiting for assembly.
	if _, err := access.New(c.accountSpecs()); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	// Referential integrity, also offline: a typo in an admit list fails the syntax
	// check rather than silently admitting no one.
	for i, ec := range c.Endpoints {
		for _, name := range ec.Admit {
			if _, ok := c.Accounts[name]; !ok {
				return nil, fmt.Errorf("config: endpoints[%d]: admit names undefined account %q", i, name)
			}
		}
		// The transport's address form, also offline: an overlong socket path, or one
		// missing its "unix://" scheme, fails the syntax check rather than the launch after it.
		if raw, ok := ec.Transport["addr"]; ok {
			var addr string
			if err := json.Unmarshal(raw, &addr); err == nil {
				if err := rest_http.ValidateAddr(addr); err != nil {
					return nil, fmt.Errorf("config: endpoints[%d].transport.addr: %w", i, err)
				}
			}
		}
	}
	// Seed the secret-store holder: it builds lazily on the first vault() reference and
	// caches each resolved secret for vaultTTL.
	c.box = newVaultBox(c.Vault)
	return &c, nil
}

// accountSpecs flattens the roster to the map the access checker consumes.
func (c *Config) accountSpecs() map[string][]string {
	specs := make(map[string][]string, len(c.Accounts))
	for name, ac := range c.Accounts {
		specs[name] = ac.Grants
	}
	return specs
}

// build assembles in dependency order: storage, signer, the sequencer that needs
// both, then each endpoint's own core. An absent section yields a nil adapter. dev
// requires sequencer.type "dev" and, when true, mints the steerable clock the
// sequencer mints claims from and POST /dev/clock later steers.
func (c *Config) build(ctx context.Context, dev bool) (*App, error) {
	var app App

	if len(c.Storage) > 0 {
		st, err := storage.New(ctx, c.section(c.Storage))
		if err != nil {
			return nil, err
		}
		app.Storage = st
	}

	if len(c.Signer) > 0 {
		sg, err := signer.New(ctx, c.section(c.Signer))
		if err != nil {
			return nil, err
		}
		app.Signer = sg
	}

	if len(c.Sequencer) > 0 {
		sec := c.section(c.Sequencer)
		// Checked here rather than in the storage port: a tree holding no bookmark
		// store is only a fault for a deployment that runs a sequencer over it.
		if app.Storage != nil {
			if err := storage.CheckBookmarks(ctx, c.section(c.Storage), app.Storage); err != nil {
				return nil, err
			}
		}
		var now func() time.Time
		if dev {
			t, err := sec.Get(ctx, "type")
			if err != nil {
				return nil, err
			}
			if t != "dev" {
				return nil, fmt.Errorf("config: --dev requires sequencer.type \"dev\", got %q", t)
			}
			app.DevClock = sequencer.NewSteerableClock()
			now = app.DevClock.Now
		}
		seq, err := sequencer.New(ctx, sec, app.Storage, app.Signer, now)
		if err != nil {
			return nil, err
		}
		app.Sequencer = seq
	}

	if len(c.Storage) > 0 {
		layers, err := storage.Describe(ctx, c.section(c.Storage))
		if err != nil {
			return nil, err
		}
		app.Layers = layers
	}

	for i, ec := range c.Endpoints {
		cr, err := c.buildEndpoint(ctx, ec, &app)
		if err != nil {
			return nil, fmt.Errorf("config: endpoints[%d]: %w", i, err)
		}
		ep, err := endpoints.New(ctx, c.section(ec.Transport), cr)
		if err != nil {
			return nil, fmt.Errorf("config: endpoints[%d]: %w", i, err)
		}
		app.Endpoints = append(app.Endpoints, ep)
	}

	return &app, nil
}

// buildEndpoint composes one endpoint's core: its auth backends indexed for scheme
// dispatch, and a checker over the accounts it admits alone.
func (c *Config) buildEndpoint(ctx context.Context, ec endpointConfig, app *App) (*core.Core, error) {
	var auths []auth.Auth
	for j, a := range ec.Auth {
		au, err := auth.New(ctx, c.section(a))
		if err != nil {
			return nil, fmt.Errorf("auth[%d]: %w", j, err)
		}
		auths = append(auths, au)
	}
	set, err := auth.NewSet(auths)
	if err != nil {
		return nil, err
	}

	admitted := make(map[string][]string, len(ec.Admit))
	for _, name := range ec.Admit {
		admitted[name] = c.Accounts[name].Grants // presence guaranteed by load's admit check
	}
	chk, err := access.New(admitted)
	if err != nil {
		return nil, err
	}

	layers := make([]core.StorageLayer, 0, len(app.Layers))
	for _, l := range app.Layers {
		layers = append(layers, core.StorageLayer{Name: l.Name, Type: l.Type})
	}
	opts := []core.Option{
		core.WithSigner(app.Signer),
		core.WithLayers(layers),
	}
	if app.DevClock != nil {
		opts = append(opts, core.WithDevClock(app.DevClock.Advance))
	}
	return core.New(set, chk, app.Sequencer, app.Storage, opts...), nil
}

// resolveAll resolves every env()/vault() reference and assembles nothing, failing on
// the first unresolvable one. The vault is dialed only if a section references it.
func (c *Config) resolveAll(ctx context.Context) error {
	singles := []struct {
		name string
		sec  section
	}{
		{"signer", c.Signer},
		{"storage", c.Storage},
		{"sequencer", c.Sequencer},
	}
	for _, s := range singles {
		if len(s.sec) == 0 {
			continue
		}
		if err := resolveSection(ctx, c.section(s.sec)); err != nil {
			return fmt.Errorf("config: %s: %w", s.name, err)
		}
	}
	// Each endpoint's transport and auth backends carry delegations; the account
	// roster and admit lists are pure policy with none.
	for i, ec := range c.Endpoints {
		if len(ec.Transport) > 0 {
			if err := resolveSection(ctx, c.section(ec.Transport)); err != nil {
				return fmt.Errorf("config: endpoints[%d].transport: %w", i, err)
			}
		}
		for j, a := range ec.Auth {
			if err := resolveSection(ctx, c.section(a)); err != nil {
				return fmt.Errorf("config: endpoints[%d].auth[%d]: %w", i, j, err)
			}
		}
	}
	return nil
}
