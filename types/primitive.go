package types

import (
	"bytes"
	"encoding/json"
)

// Primitive preserves a JSON number, string, boolean or null without coercion.
// The zero value denotes an absent optional field; explicit null is distinct.
type Primitive struct{ raw string }

func (v Primitive) String() string { return v.raw }
func (v Primitive) IsZero() bool   { return v.raw == "" }
func (v Primitive) MarshalJSON() ([]byte, error) {
	if v.raw == "" {
		return []byte("null"), nil
	}
	return []byte(v.raw), nil
}
func (v *Primitive) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if !json.Valid(raw) || len(raw) == 0 || raw[0] == '{' || raw[0] == '[' {
		return NewError(ProtocolError)
	}
	v.raw = string(raw)
	return nil
}
