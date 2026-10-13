package types

import (
	"regexp"
)

var integerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

// Integer retains arbitrary-precision provider integers without floating point.
type Integer struct{ raw string }

func ParseInteger(raw string) (Integer, error) {
	if !integerPattern.MatchString(raw) {
		return Integer{}, NewError(InvalidArgument)
	}
	return Integer{raw: raw}, nil
}

func (i Integer) String() string { return i.raw }
func (i Integer) MarshalJSON() ([]byte, error) {
	if i.raw == "" {
		return []byte("null"), nil
	}
	return []byte(i.raw), nil
}
func (i *Integer) UnmarshalJSON(raw []byte) error {
	if !integerPattern.Match(raw) {
		return NewError(ProtocolError)
	}
	i.raw = string(raw)
	return nil
}
