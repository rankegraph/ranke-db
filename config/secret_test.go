package config

import (
	"bytes"
	"io"
	"testing"

	"filippo.io/age"
)

// encryptForTest age-encrypts plaintext under a scrypt passphrase, returning the
// binary age stream — the format an operator commits when a config carries an
// inline secret.
func encryptForTest(t *testing.T, plaintext, passphrase string) []byte {
	t.Helper()
	recip, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		t.Fatalf("NewScryptRecipient: %v", err)
	}
	recip.SetWorkFactor(10) // keep the test fast
	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, recip)
	if err != nil {
		t.Fatalf("age.Encrypt: %v", err)
	}
	if _, err := io.WriteString(w, plaintext); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return buf.Bytes()
}

// TestDecryptRoundTrip encrypts a config and decrypts it through a passphrase
// source, asserting the plaintext returns intact.
func TestDecryptRoundTrip(t *testing.T) {
	const plaintext = `{"accounts":{"ops":{"grants":["R foo_*"]}}}`
	const pass = "correct horse battery staple"
	enc := encryptForTest(t, plaintext, pass)

	src := PassphraseSource(func() (string, error) { return pass, nil })
	got, err := decrypt(enc, src)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if string(got) != plaintext {
		t.Fatalf("decrypted = %q, want %q", got, plaintext)
	}
}

// TestDecryptPlaintextPassthrough asserts a plaintext config is returned
// unchanged and needs no key source.
func TestDecryptPlaintextPassthrough(t *testing.T) {
	plain := []byte(`{"accounts":{"ops":{"grants":["R foo_*"]}}}`)
	got, err := decrypt(plain, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("plaintext passthrough = %q, want %q", got, plain)
	}
}

// TestDecryptEncryptedNoKey asserts an encrypted config with no key source fails
// loud rather than silently.
func TestDecryptEncryptedNoKey(t *testing.T) {
	enc := encryptForTest(t, `{}`, "pw")
	if _, err := decrypt(enc, nil); err == nil {
		t.Fatal("decrypt of encrypted config with nil source = nil error, want error")
	}
}
