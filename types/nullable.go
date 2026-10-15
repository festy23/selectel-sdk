package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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

// Format uses the public JSON representation, so protected values nested in T
// retain their encoding boundary instead of exposing unexported storage bytes.
func (n Nullable[T]) Format(state fmt.State, _ rune) {
	raw, err := n.MarshalJSON()
	if err != nil {
		_, _ = io.WriteString(state, redacted)
		return
	}
	_, _ = state.Write(raw)
}
func (n Nullable[T]) MarshalText() ([]byte, error) { return n.MarshalJSON() }

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
