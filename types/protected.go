// Package types contains the shared public contracts of selectelsdk.
package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const redacted = "[REDACTED]"

// ProtectedValue prevents accidental disclosure through formatting and encoding.
// Reveal and CopyTo are explicit disclosure points. They do not guarantee erasure
// of copies retained by Go or by the caller.
type ProtectedValue struct{ value []byte }

func Protect(value []byte) ProtectedValue {
	return ProtectedValue{value: bytes.Clone(value)}
}

func (v ProtectedValue) Reveal() []byte               { return bytes.Clone(v.value) }
func (v ProtectedValue) Empty() bool                  { return len(v.value) == 0 }
func (v ProtectedValue) String() string               { return redacted }
func (v ProtectedValue) GoString() string             { return redacted }
func (v ProtectedValue) Format(s fmt.State, _ rune)   { _, _ = io.WriteString(s, redacted) }
func (v ProtectedValue) MarshalJSON() ([]byte, error) { return json.Marshal(redacted) }
func (v ProtectedValue) MarshalText() ([]byte, error) { return []byte(redacted), nil }

// CopyTo discloses the contents only to the explicitly supplied writer.
func (v ProtectedValue) CopyTo(w io.Writer) (int64, error) {
	if w == nil {
		return 0, NewError(InvalidArgument)
	}
	return io.Copy(w, bytes.NewReader(v.value))
}
