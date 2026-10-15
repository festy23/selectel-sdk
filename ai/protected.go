package ai

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/festy23/selectel-sdk/types"
)

// Vector preserves exact numeric outputs while keeping model-derived content out
// of diagnostics and serialization. Reveal returns a caller-owned copy.
type Vector struct{ values []types.Number }

func (v Vector) Reveal() []types.Number     { return append([]types.Number(nil), v.values...) }
func (Vector) String() string               { return "[REDACTED]" }
func (Vector) GoString() string             { return "[REDACTED]" }
func (Vector) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[REDACTED]") }
func (Vector) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }
func (Vector) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }
func (v *Vector) UnmarshalJSON(raw []byte) error {
	v.values = nil
	var values []types.Number
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return types.NewError(types.ProtocolError)
	}
	v.values = values
	return nil
}

// ProtectedNumber retains an exact model-derived numeric output.
type ProtectedNumber struct{ value types.Number }

func (v ProtectedNumber) Reveal() types.Number       { return v.value }
func (ProtectedNumber) String() string               { return "[REDACTED]" }
func (ProtectedNumber) GoString() string             { return "[REDACTED]" }
func (ProtectedNumber) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[REDACTED]") }
func (ProtectedNumber) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }
func (ProtectedNumber) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }
func (v *ProtectedNumber) UnmarshalJSON(raw []byte) error {
	v.value = types.Number{}
	var n types.Number
	if n.UnmarshalJSON(raw) != nil {
		return types.NewError(types.ProtocolError)
	}
	v.value = n
	return nil
}

// Document distinguishes the two confirmed text document representations.
// Image documents are outside this text-only request binding.
type Document struct {
	Form string
	Text types.ProtectedValue
}

func (v *Document) UnmarshalJSON(raw []byte) error {
	*v = Document{}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		v.Form = "string"
		v.Text = types.Protect([]byte(text))
		return nil
	}
	var object struct {
		Text *string `json:"text"`
	}
	if json.Unmarshal(raw, &object) != nil || object.Text == nil {
		return types.NewError(types.ProtocolError)
	}
	v.Form = "object"
	v.Text = types.Protect([]byte(*object.Text))
	return nil
}
func (Document) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }

// TokenIDs protects tokenized model output while retaining exact integers.
type TokenIDs struct{ values []types.Integer }

func (v TokenIDs) Reveal() []types.Integer    { return append([]types.Integer(nil), v.values...) }
func (TokenIDs) String() string               { return "[REDACTED]" }
func (TokenIDs) GoString() string             { return "[REDACTED]" }
func (TokenIDs) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, "[REDACTED]") }
func (TokenIDs) MarshalJSON() ([]byte, error) { return []byte(`"[REDACTED]"`), nil }
func (TokenIDs) MarshalText() ([]byte, error) { return []byte("[REDACTED]"), nil }
func (v *TokenIDs) UnmarshalJSON(raw []byte) error {
	v.values = nil
	var values []types.Integer
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return types.NewError(types.ProtocolError)
	}
	v.values = values
	return nil
}
