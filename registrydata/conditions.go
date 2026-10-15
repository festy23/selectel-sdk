package registrydata

import (
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"strings"
)

// entityTag checks RFC9110 entity-tag bytes; opaque tags are not quoted-strings:
// backslashes are literal, commas inside quotes do not delimit multiple tags.
func entityTag(raw string) bool {
	if strings.HasPrefix(raw, "W/") {
		raw = raw[2:]
	}
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return false
	}
	for _, b := range []byte(raw[1 : len(raw)-1]) {
		if b != 0x21 && !(b >= 0x23 && b <= 0x7e) && b < 0x80 {
			return false
		}
	}
	return true
}
func responseETag(h http.Header, limit uint64) (*types.ProtectedValue, error) {
	raw, e := header(h, "ETag", limit)
	if e != nil {
		return nil, e
	}
	if len(h.Values("ETag")) == 0 {
		return nil, nil
	}
	if !entityTag(raw) {
		return nil, types.NewError(types.ProtocolError)
	}
	value := types.Protect([]byte(raw))
	return &value, nil
}
func conditionHeaders(h http.Header, in ManifestWriteOptions, limit uint64) error {
	var total uint64
	for _, item := range []struct {
		name  string
		value *EntityTagCondition
	}{{"If-Match", in.IfMatch}, {"If-None-Match", in.IfNoneMatch}} {
		if item.value == nil {
			continue
		}
		value := item.value
		if value.Any {
			if len(value.Tags) != 0 {
				return types.NewError(types.InvalidArgument)
			}
			total++
			if total > limit {
				return types.NewError(types.InvalidArgument)
			}
			h.Set(item.name, "*")
			continue
		}
		if len(value.Tags) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		parts := []string{}
		for i, tag := range value.Tags {
			raw := string(tag.Reveal())
			total += uint64(len(raw))
			if i != 0 {
				total += 2
			}
			if total > limit || !entityTag(raw) {
				return types.NewError(types.InvalidArgument)
			}
			parts = append(parts, raw)
		}
		h.Set(item.name, strings.Join(parts, ", "))
	}
	return nil
}
