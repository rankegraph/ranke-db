// package: signer / crypto
// type:    logic
// job:     present the port as a crypto.Signer, for the libraries that take one
// limits:  carries a context, which crypto.Signer has no room for (-> signer.go)
//
// ranke-go signs a claim through a crypto.Signer, so an in-process key and one that
// never leaves a vault reach it the same way: the signature is a call either way.
package signer

import (
	"context"
	"crypto"
	"fmt"
	"io"
)

// CryptoSigner adapts s for a library that takes a crypto.Signer, reading the public half
// once. ctx bounds every signature it makes.
func CryptoSigner(ctx context.Context, s Signer) (crypto.Signer, error) {
	pub, err := s.Public(ctx)
	if err != nil {
		return nil, fmt.Errorf("signer: public key: %w", err)
	}
	return portKey{ctx: ctx, sig: s, pub: pub}, nil
}

// portKey is that adaptation.
type portKey struct {
	ctx context.Context
	sig Signer
	pub crypto.PublicKey
}

// Public returns the public half of the identity.
func (k portKey) Public() crypto.PublicKey { return k.pub }

// Sign signs the digest through the port; the backends hold their own entropy.
func (k portKey) Sign(_ io.Reader, digest []byte, _ crypto.SignerOpts) ([]byte, error) {
	return k.sig.Sign(k.ctx, digest)
}
