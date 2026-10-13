package types_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/festy23/selectel-sdk/types"
)

func TestProtectedValueDisclosesOnlyExplicitly(t *testing.T) {
	input := []byte("synthetic-private-marker")
	v := types.Protect(input)
	input[0] = 'X'
	for _, f := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
		if got := fmt.Sprintf(f, v); got != "[REDACTED]" {
			t.Fatalf("format %s did not redact", f)
		}
	}
	b, err := json.Marshal(struct {
		Value types.ProtectedValue `json:"value"`
	}{v})
	if err != nil || bytes.Contains(b, []byte("private-marker")) {
		t.Fatal("JSON leaked")
	}
	revealed := v.Reveal()
	revealed[0] = 'Y'
	if got := string(v.Reveal()); got != "synthetic-private-marker" {
		t.Fatal("shared disclosure buffer")
	}
	var dst bytes.Buffer
	if _, err := v.CopyTo(&dst); err != nil || dst.String() != "synthetic-private-marker" {
		t.Fatal("explicit copy failed")
	}
}

func TestSanitizedContextErrorSupportsMatching(t *testing.T) {
	err := types.ContextError(fmt.Errorf("synthetic-private-marker: %w", context.Canceled))
	if !errors.Is(err, context.Canceled) || !errors.Is(err, types.NewError(types.Canceled)) {
		t.Fatal("matching lost")
	}
	data, e := json.Marshal(err)
	if e != nil || strings.Contains(string(data), "private-marker") || strings.Contains(fmt.Sprintf("%#v", err), "private-marker") {
		t.Fatal("raw cause leaked")
	}
}

func TestEndpointRejectsCredentialDestinations(t *testing.T) {
	for _, raw := range []string{"http://api.example.test", "https://user:pass@api.example.test", "https://api.example.test?token=synthetic", "https://api.example.test#fragment", "https:///missing"} {
		if types.ValidateEndpoint(raw) == nil {
			t.Fatal("unsafe endpoint accepted")
		}
	}
}
