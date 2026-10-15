package onec

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"strconv"
)

// ProtectedNumber retains the exact source JSON number without exposing raw
// telemetry through formatting or encoding. Reveal is an explicit caller boundary.
type ProtectedNumber struct{ types.ProtectedValue }

func (v *ProtectedNumber) UnmarshalJSON(raw []byte) error {
	if _, err := types.ParseNumber(string(raw)); err != nil {
		return err
	}
	v.ProtectedValue = types.Protect(raw)
	return nil
}

// ProtectedInt64String and ProtectedUint64String retain source integer-string
// telemetry exactly while protecting every ordinary diagnostic representation.
type ProtectedInt64String struct{ types.ProtectedValue }
type ProtectedUint64String struct{ types.ProtectedValue }

func (v ProtectedInt64String) Validate() error {
	if _, e := strconv.ParseInt(string(v.Reveal()), 10, 64); e != nil {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
func (v ProtectedUint64String) Validate() error {
	if _, e := strconv.ParseUint(string(v.Reveal()), 10, 64); e != nil {
		return types.NewError(types.ProtocolError)
	}
	return nil
}
func (v *ProtectedInt64String) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	v.ProtectedValue = types.Protect([]byte(text))
	return v.Validate()
}
func (v *ProtectedUint64String) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	v.ProtectedValue = types.Protect([]byte(text))
	return v.Validate()
}
