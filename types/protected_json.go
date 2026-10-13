package types

import "encoding/json"

// ProtectedJSON preserves schemaless provider data without putting it in logs.
// Reveal returns the original JSON bytes for an explicit caller-side decode.
type ProtectedJSON struct{ ProtectedValue }

func (v *ProtectedJSON) UnmarshalJSON(raw []byte) error {
	if !json.Valid(raw) {
		return NewError(ProtocolError)
	}
	v.ProtectedValue = Protect(raw)
	return nil
}

// UnmarshalJSON reads a provider string into a protected value. Serializing it
// still redacts it; serialized redactions cannot restore the original value.
func (v *ProtectedValue) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return NewError(ProtocolError)
	}
	*v = Protect([]byte(text))
	return nil
}
