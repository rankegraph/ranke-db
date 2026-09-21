package config

import (
	"context"
	"strings"
	"testing"

	"github.com/rankegraph/ranke-go"
)

// TestBuildStorageStack assembles a two-layer stack (mem over fs) from a single
// storage descriptor and round-trips content through it, proving the storage
// section resolves into a working Universe with its layers composed in order.
// Neither layer declares a mode: since ranke-go v0.3.0 the write tier belongs to
// the backend (both of these are authoritative), not to the composition.
func TestBuildStorageStack(t *testing.T) {
	dir := t.TempDir()
	cfgJSON := `{
		"storage": {
			"type": "stack",
			"layers": [
				{"type": "mem"},
				{"type": "fs", "dir": "` + dir + `"}
			]
		}
	}`

	app, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, false)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if app.Storage == nil {
		t.Fatal("nil storage universe")
	}
	t.Cleanup(func() { _ = app.Storage.Close() })

	ctx := context.Background()
	content := []byte("ranke storage stack round-trip")
	hash, err := ranke.HashContent(content)
	if err != nil {
		t.Fatalf("HashContent: %v", err)
	}
	if err := ranke.PutContent(ctx, app.Storage, hash, content); err != nil {
		t.Fatalf("PutContent: %v", err)
	}
	got, err := app.Storage.GetContents(ctx, []ranke.ContentRef{{Hash: hash, ContentSize: uint64(len(content))}})
	if err != nil {
		t.Fatalf("GetContents: %v", err)
	}
	if len(got) != 1 || string(got[0]) != string(content) {
		t.Fatalf("round-trip = %q, want %q", got, content)
	}
}

// TestBuildStorageStackRejectsUnsettableLayer covers the layer knobs a backend
// cannot honour. A mem universe is authoritative and uncapped, and ranke-go
// exposes neither as a stack option, so asking for a lazy cache or a size cap
// must fail the launch — accepting it would silently store every write in a
// layer the operator meant to be a pass-through cache.
func TestBuildStorageStackRejectsUnsettableLayer(t *testing.T) {
	for _, tc := range []struct{ name, layer, want string }{
		{"mode", `{"type": "mem", "mode": "lazy"}`, `tier`},
		{"maxContentSize", `{"type": "mem", "maxContentSize": "8kb"}`, `caps content at`},
		{"noReadFill", `{"type": "mem", "noReadFill": "true"}`, `remove the key`},
		{"unknown mode", `{"type": "mem", "mode": "warm"}`, `unknown "warm"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfgJSON := `{"storage": {"type": "stack", "layers": [` + tc.layer + `]}}`
			app, err := Run(context.Background(), strings.NewReader(cfgJSON), nil, false)
			if err == nil {
				t.Cleanup(func() { _ = app.Storage.Close() })
				t.Fatalf("Run accepted %s, want a refusal", tc.layer)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}
