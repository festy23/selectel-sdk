package types

import (
	"bytes"
	"encoding/json"
)

// Nullable distinguishes an explicit JSON null from a present value.
// A nil *Nullable in an omitempty request field omits that field altogether.
type Nullable[T any] struct {
	value T
	valid bool
}

func Null[T any]() *Nullable[T]         { return &Nullable[T]{} }
func Value[T any](value T) *Nullable[T] { return &Nullable[T]{value: value, valid: true} }
func (n Nullable[T]) Value() (T, bool)  { return n.value, n.valid }
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if !n.valid {
		return []byte("null"), nil
	}
	return json.Marshal(n.value)
}
func (n *Nullable[T]) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		*n = Nullable[T]{}
		return nil
	}
	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	*n = Nullable[T]{value: value, valid: true}
	return nil
}
