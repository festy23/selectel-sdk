package swift

import (
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/types"
)

var metadataKey = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")

func setProtected(h http.Header, key string, v *types.ProtectedValue) error {
	if v == nil {
		return nil
	}
	raw := v.Reveal()
	if !utf8.Valid(raw) || strings.ContainsAny(string(raw), "\r\n\x00") {
		return types.NewError(types.InvalidArgument)
	}
	for _, b := range raw {
		if (b < 32 && b != '\t') || b == 127 {
			return types.NewError(types.InvalidArgument)
		}
	}
	h.Set(key, string(raw))
	return nil
}
func metadataRequest(prefix string, set map[string]types.ProtectedValue, remove []string) (http.Header, error) {
	h := http.Header{}
	seen := map[string]bool{}
	for k, v := range set {
		reserved := map[string]bool{}
		if prefix == "X-Account-Meta-" {
			reserved["quota-bytes"] = true
		}
		if prefix == "X-Container-Meta-" {
			for _, name := range []string{"quota-bytes", "quota-count", "type", "default-delete-after"} {
				reserved[name] = true
			}
		}
		if !metadataKey.MatchString(k) || seen[strings.ToLower(k)] || reserved[strings.ToLower(k)] {
			return h, types.NewError(types.InvalidArgument)
		}
		seen[strings.ToLower(k)] = true
		if e := setProtected(h, prefix+k, &v); e != nil {
			return h, e
		}
	}
	for _, k := range remove {
		if !metadataKey.MatchString(k) || seen[strings.ToLower(k)] {
			return h, types.NewError(types.InvalidArgument)
		}
		seen[strings.ToLower(k)] = true
		h.Set("X-Remove-"+strings.TrimPrefix(prefix, "X-")+k, "true")
	}
	return h, nil
}
func setInteger(h http.Header, key string, v *types.Integer) error {
	if v == nil {
		return nil
	}
	if h.Get("X-Remove-"+strings.TrimPrefix(key, "X-")) != "" {
		return types.NewError(types.InvalidArgument)
	}
	if _, e := types.ParseInteger(v.String()); e != nil {
		return types.NewError(types.InvalidArgument)
	}
	h.Set(key, v.String())
	return nil
}
func setBoolean(h http.Header, key string, v *bool) {
	if v != nil {
		value := "false"
		if *v {
			value = "true"
		}
		h.Set(key, value)
	}
}
func accountInfo(h http.Header, ref *types.ResourceRef) (AccountInfo, error) {
	out := AccountInfo{Ref: ref, Metadata: metadata(h, "X-Account-Meta-"), StoragePolicies: map[string]StoragePolicyUsage{}}
	var e error
	for _, f := range []struct {
		key string
		v   **types.Integer
	}{{"X-Account-Bytes-Used", &out.BytesUsed}, {"X-Account-Container-Count", &out.ContainerCount}, {"X-Account-Object-Count", &out.ObjectCount}, {"X-Account-Meta-Quota-Bytes", &out.QuotaBytes}} {
		*f.v, e = integer(h, f.key)
		if e != nil {
			return out, e
		}
	}
	out.Created, e = number(h, "X-Timestamp")
	if e != nil {
		return out, e
	}
	prefix := "X-Account-Storage-Policy-"
	for key := range h {
		if !strings.HasPrefix(strings.ToLower(key), strings.ToLower(prefix)) {
			continue
		}
		for _, suffix := range []string{"-Bytes-Used", "-Container-Count", "-Object-Count"} {
			if !strings.HasSuffix(strings.ToLower(key), strings.ToLower(suffix)) {
				continue
			}
			name := key[len(prefix) : len(key)-len(suffix)]
			if name == "" {
				return out, types.NewError(types.ProtocolError)
			}
			n, e := integer(h, key)
			if e != nil {
				return out, e
			}
			policy := out.StoragePolicies[name]
			switch suffix {
			case "-Bytes-Used":
				policy.BytesUsed = n
			case "-Container-Count":
				policy.ContainerCount = n
			case "-Object-Count":
				policy.ObjectCount = n
			}
			out.StoragePolicies[name] = policy
		}
	}
	return out, nil
}
func bucketInfo(h http.Header, ref *types.ResourceRef) (BucketInfo, error) {
	out := BucketInfo{Ref: ref, Metadata: metadata(h, "X-Container-Meta-"), Type: value(h, "X-Container-Meta-Type"), StoragePolicyName: value(h, "X-Container-Storage-Policy-Name")}
	var e error
	for _, f := range []struct {
		key string
		v   **types.Integer
	}{{"X-Container-Bytes-Used", &out.BytesUsed}, {"X-Container-Object-Count", &out.ObjectCount}, {"X-Container-Meta-Quota-Bytes", &out.QuotaBytes}, {"X-Container-Meta-Quota-Count", &out.QuotaCount}, {"X-Container-Meta-Default-Delete-After", &out.DefaultDeleteAfter}, {"X-Container-Storage-Policy-Index", &out.StoragePolicyIndex}} {
		*f.v, e = integer(h, f.key)
		if e != nil {
			return out, e
		}
	}
	out.Created, e = number(h, "X-Timestamp")
	if e != nil {
		return out, e
	}
	out.VersionsEnabled, e = boolean(h, "X-Versions-Enabled")
	return out, e
}
func objectInfo(h http.Header, ref *types.ResourceRef) (ObjectInfo, error) {
	out := ObjectInfo{Ref: ref, ETag: value(h, "ETag"), LastModifiedRaw: value(h, "Last-Modified"), Metadata: metadata(h, "X-Object-Meta-"), Manifest: protected(h, "X-Object-Manifest"), SymlinkTarget: protected(h, "X-Symlink-Target"), SymlinkTargetETag: value(h, "X-Symlink-Target-Etag"), CopiedFrom: protected(h, "X-Copied-From"), CopiedFromLastModifiedRaw: value(h, "X-Copied-From-Last-Modified")}
	var e error
	out.ContentType = protected(h, "Content-Type")
	out.ContentEncoding = protected(h, "Content-Encoding")
	out.ContentDisposition = protected(h, "Content-Disposition")
	out.CacheControl = protected(h, "Cache-Control")
	out.Link = protected(h, "Link")
	out.ContentLength, e = integer(h, "Content-Length")
	if e != nil {
		return out, e
	}
	out.DeleteAt, e = integer(h, "X-Delete-At")
	if e != nil {
		return out, e
	}
	out.Created, e = number(h, "X-Timestamp")
	if e != nil {
		return out, e
	}
	out.StaticLargeObject, e = boolean(h, "X-Static-Large-Object")
	return out, e
}

func setText(h http.Header, key string, v *string) error {
	if v == nil {
		return nil
	}
	p := types.Protect([]byte(*v))
	return setProtected(h, key, &p)
}
