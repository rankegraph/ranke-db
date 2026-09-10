package contributor

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rankegraph/ranke-go/keysource"
)

// keyPEM writes a throwaway Ed25519 key in the PKCS#8 PEM form the flag takes.
func keyPEM(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
}

// TestLoadYieldsAUsableIdentity covers the seam this package is: keysource reads the
// bytes, ParseKeypair turns them into an identity carrying the multikey public half a
// contributor claim needs. The grammar's own cases are the library's to test.
func TestLoadYieldsAUsableIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contributor.pem")
	if err := os.WriteFile(path, []byte(keyPEM(t)), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	pair, err := Load(path, nil)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if pair.Private == nil {
		t.Error("no private key")
	}
	if len(pair.Pubkey) == 0 {
		t.Error("no multikey public half, which a contributor claim carries as its content")
	}
}

// TestLoadCarriesTheRefusals: the two that protect a key reach the caller rather than
// being swallowed here, so swapping the call or dropping an option is caught.
func TestLoadCarriesTheRefusals(t *testing.T) {
	t.Run("key in plain", func(t *testing.T) {
		_, err := Load(keyPEM(t), nil)
		if !errors.Is(err, keysource.ErrInline) {
			t.Fatalf("Load(literal key) = %v, want ErrInline", err)
		}
		if !strings.Contains(err.Error(), "rotate") {
			t.Errorf("the refusal should say to rotate: %v", err)
		}
	})
	t.Run("readable by others", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "contributor.pem")
		if err := os.WriteFile(path, []byte(keyPEM(t)), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := Load(path, nil); !errors.Is(err, keysource.ErrPermission) {
			t.Fatalf("Load(0644 key) = %v, want ErrPermission", err)
		}
	})
}

// TestLoadAllowsAPrompt: this is a tool a person runs, so the terminal spelling is
// granted. A server passing no WithTTY gets ErrNoTTY instead, which is the point of the
// option — the refusal here would be that error, never a silent read.
func TestLoadAllowsAPrompt(t *testing.T) {
	_, err := Load("prompt", strings.NewReader(keyPEM(t)))
	if errors.Is(err, keysource.ErrNoTTY) {
		t.Fatal("prompt was refused; this binary grants WithTTY")
	}
}
