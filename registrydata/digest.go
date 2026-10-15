package registrydata

import (
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"github.com/festy23/selectel-sdk/types"
	"strings"
)

type Digest struct {
	Algorithm string `json:"algorithm"`
	Encoded   string `json:"encoded"`
}

func (d Digest) String() string { return d.Algorithm + ":" + d.Encoded }
func ParseDigest(raw string) (Digest, error) {
	a, b, ok := strings.Cut(raw, ":")
	if !ok || a == "" || b == "" {
		return Digest{}, types.NewError(types.InvalidArgument)
	}
	n := 0
	switch a {
	case "sha256":
		n = 64
	case "sha384":
		n = 96
	case "sha512":
		n = 128
	default:
		return Digest{}, types.NewError(types.CapabilityUnavailable)
	}
	decoded, e := hex.DecodeString(b)
	if !ok || e != nil || len(decoded)*2 != n || b != strings.ToLower(b) {
		return Digest{}, types.NewError(types.InvalidArgument)
	}
	return Digest{a, b}, nil
}
func hashBytes(raw []byte, algorithm string) (Digest, error) {
	switch algorithm {
	case "sha256":
		s := sha256.Sum256(raw)
		return Digest{"sha256", hex.EncodeToString(s[:])}, nil
	case "sha384":
		s := sha512.Sum384(raw)
		return Digest{"sha384", hex.EncodeToString(s[:])}, nil
	case "sha512":
		s := sha512.Sum512(raw)
		return Digest{"sha512", hex.EncodeToString(s[:])}, nil
	}
	return Digest{}, types.NewError(types.CapabilityUnavailable)
}
