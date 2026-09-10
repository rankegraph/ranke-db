// package: contributor / cmd
// type:    logic
// job:     resolve a --pubkey argument to the multikey bytes a contributor claim carries
// limits:  parsing only; the framing and the PEM are the library's (-> ranke-go sign)
//
// A pubkey is public, so it is admitted on the command line, where a signing key never is
// (-> load.go): it travels from the newcomer to whoever admits them, by whatever channel
// they already have.
package contributor

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/rankegraph/ranke-go"
)

// ErrPubkey is a --pubkey argument that names neither form.
var ErrPubkey = errors.New("ranke-client: unrecognised pubkey")

// Pubkey resolves spec to the multikey-encoded key: the hex `contributor list` prints, or
// the path to an Ed25519 public-key PEM as `openssl pkey -pubout` writes one. Both are
// checked against the framing `V-SIGN` fixes, so a key that could sign nothing is refused
// here rather than by the server after a contribution was built.
func Pubkey(spec string) ([]byte, error) {
	if key, err := fromHex(spec); err == nil {
		return key, nil
	}
	pemBytes, err := os.ReadFile(spec)
	if err != nil {
		return nil, fmt.Errorf("%w: %q is neither a multikey in hex nor a readable file", ErrPubkey, spec)
	}
	pub, err := ranke.ParseEd25519PublicKeyPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: %q holds no Ed25519 public key: %w", ErrPubkey, spec, err)
	}
	return ranke.EncodePublicKey(pub)
}

// fromHex reads the listing's own form, rejecting bytes that carry no known scheme.
func fromHex(spec string) ([]byte, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(spec))
	if err != nil {
		return nil, err
	}
	if _, _, err := ranke.DecodePublicKey(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
