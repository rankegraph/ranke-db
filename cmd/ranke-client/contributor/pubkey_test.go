package contributor

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/rankegraph/ranke-go"
)

// TestPubkeyReadsBothForms: the hex a listing prints and the PEM `openssl pkey -pubout`
// writes name the same key, so whichever the newcomer sent resolves to one multikey.
func TestPubkeyReadsBothForms(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	want, err := ranke.EncodePublicKey(pub)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(t.TempDir(), "key.pub.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, tc := range []struct{ name, spec string }{
		{"hex, as the listing prints it", hex.EncodeToString(want)},
		{"hex with a stray newline", hex.EncodeToString(want) + "\n"},
		{"a public-key PEM", path},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Pubkey(tc.spec)
			if err != nil {
				t.Fatalf("Pubkey: %v", err)
			}
			if string(got) != string(want) {
				t.Errorf("resolved %x, want the multikey %x", got, want)
			}
		})
	}
}

// TestPubkeyRefusesWhatCouldSignNothing: an argument reaching the claim builder unchecked
// would be refused after the read and the build, naming the wrong step.
func TestPubkeyRefusesWhatCouldSignNothing(t *testing.T) {
	for _, tc := range []struct{ name, spec string }{
		{"empty", ""},
		{"hex of the wrong length", hex.EncodeToString([]byte{0xed, 0x01, 0x02})},
		{"an unknown scheme", hex.EncodeToString(append([]byte{0x99, 0x01}, make([]byte, 32)...))},
		{"a name that is neither hex nor a file", "not-a-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Pubkey(tc.spec); err == nil {
				t.Error("accepted, want a refusal")
			}
		})
	}
}
