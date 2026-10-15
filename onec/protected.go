package onec

import "github.com/festy23/selectel-sdk/types"

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
