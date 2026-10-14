package dedicated

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Optional distinguishes provider null/value; a nil pointer denotes omission.
// Formatting is redacted even when T encloses a protected byte slice.
type Optional[T any] struct {
	value T
	valid bool
}

func Value[T any](v T) *Optional[T]    { return &Optional[T]{value: v, valid: true} }
func Null[T any]() *Optional[T]        { return &Optional[T]{} }
func (v Optional[T]) Value() (T, bool) { return v.value, v.valid }
func (v *Optional[T]) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*v = Optional[T]{}
		return nil
	}
	var value T
	if e := json.Unmarshal(raw, &value); e != nil {
		return e
	}
	*v = Optional[T]{value: value, valid: true}
	return nil
}
func (v Optional[T]) MarshalJSON() ([]byte, error) {
	if !v.valid {
		return []byte("null"), nil
	}
	return json.Marshal(v.value)
}
func (Optional[T]) String() string             { return "[REDACTED]" }
func (Optional[T]) GoString() string           { return "[REDACTED]" }
func (Optional[T]) Format(s fmt.State, _ rune) { _, _ = io.WriteString(s, "[REDACTED]") }

type optionalWire interface{ wireValue() (any, error) }

func (v Optional[T]) wireValue() (any, error) {
	if !v.valid {
		return nil, nil
	}
	return wireValue(reflect.ValueOf(v.value))
}
