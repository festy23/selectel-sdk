package types

import "regexp"

// Number retains a JSON decimal literal, including subsecond timestamp precision.
type Number struct{ raw string }

var jsonNumber = regexp.MustCompile(`^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)

func ParseNumber(raw string) (Number, error) {
	if !jsonNumber.MatchString(raw) {
		return Number{}, NewError(ProtocolError)
	}
	return Number{raw: raw}, nil
}
func (n Number) String() string { return n.raw }
func (n Number) MarshalJSON() ([]byte, error) {
	if n.raw == "" {
		return []byte("null"), nil
	}
	return []byte(n.raw), nil
}
func (n *Number) UnmarshalJSON(raw []byte) error {
	value, err := ParseNumber(string(raw))
	if err != nil {
		return err
	}
	*n = value
	return nil
}
