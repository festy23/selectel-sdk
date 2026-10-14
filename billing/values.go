package billing

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"time"
)

// Amount retains exact minor units. The pinned report/transaction schemas call
// them kopecks; no currency conversion or inferred account currency is applied.
type Amount struct{ types.Money }

func (v *Amount) UnmarshalJSON(raw []byte) error {
	var n types.Integer
	if json.Unmarshal(raw, &n) != nil {
		return types.NewError(types.ProtocolError)
	}
	v.Money = types.Money{Raw: n.String(), Unit: "kopeck"}
	return nil
}

// Identifier retains the source's integer-or-string project IDs without loss.
type Identifier struct {
	Value   string `json:"value"`
	Numeric bool   `json:"numeric"`
}

func (v *Identifier) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return types.NewError(types.ProtocolError)
		}
		*v = Identifier{Value: s}
		return nil
	}
	var n types.Integer
	if json.Unmarshal(raw, &n) != nil {
		return types.NewError(types.ProtocolError)
	}
	*v = Identifier{Value: n.String(), Numeric: true}
	return nil
}

// ProviderTimestamp keeps timezone-free provider times separate from UTC.
type ProviderTimestamp struct {
	Raw string     `json:"raw"`
	UTC *time.Time `json:"utc,omitempty"`
}

func (v *ProviderTimestamp) UnmarshalJSON(raw []byte) error {
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return types.NewError(types.ProtocolError)
	}
	if t, e := time.Parse(time.RFC3339Nano, s); e == nil {
		utc := t.UTC()
		*v = ProviderTimestamp{Raw: s, UTC: &utc}
		return nil
	}
	if _, e := time.Parse("2006-01-02T15:04:05.999999999", s); e != nil {
		return types.NewError(types.ProtocolError)
	}
	*v = ProviderTimestamp{Raw: s}
	return nil
}

// OptionalText distinguishes provider null from a protected string; nil pointer
// denotes omission. Formatting never exposes the enclosed bytes.
type OptionalText struct {
	value types.ProtectedValue
	valid bool
}

func (v OptionalText) Value() (types.ProtectedValue, bool) { return v.value, v.valid }
func (v *OptionalText) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*v = OptionalText{}
		return nil
	}
	var text types.ProtectedValue
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	*v = OptionalText{value: text, valid: true}
	return nil
}
func (v OptionalText) MarshalJSON() ([]byte, error) {
	if !v.valid {
		return []byte("null"), nil
	}
	return json.Marshal(v.value)
}
func (OptionalText) String() string                 { return "[REDACTED]" }
func (OptionalText) GoString() string               { return "[REDACTED]" }
func (OptionalText) Format(state fmt.State, _ rune) { _, _ = io.WriteString(state, "[REDACTED]") }
