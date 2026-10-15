package mobilefarm

import "github.com/festy23/selectel-sdk/types"

// ProtectedNumber preserves source numeric telemetry exactly behind Reveal.
type ProtectedNumber struct{ types.ProtectedValue }

func (v *ProtectedNumber) UnmarshalJSON(raw []byte) error {
	if _, e := types.ParseNumber(string(raw)); e != nil {
		return e
	}
	v.ProtectedValue = types.Protect(raw)
	return nil
}

type ProtectedInteger struct{ types.ProtectedValue }

func (v *ProtectedInteger) UnmarshalJSON(raw []byte) error {
	if _, e := types.ParseInteger(string(raw)); e != nil {
		return e
	}
	v.ProtectedValue = types.Protect(raw)
	return nil
}
