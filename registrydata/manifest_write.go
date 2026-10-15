package registrydata

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"math/big"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func manifestPath(ref types.ResourceRef) string {
	if ref.Parent == nil || ref.Parent.Parent == nil {
		return ""
	}
	return "/v2/" + ref.Parent.Parent.ID + "/" + ref.Parent.ID + "/manifests/" + url.PathEscape(ref.ID)
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func (c *Manifests) valid(ref types.ResourceRef) error {
	if ref.Kind != "manifest_tag" && ref.Kind != "manifest_digest" {
		return types.NewError(types.InvalidArgument)
	}
	if e := c.client.valid(ref, ref.Kind); e != nil {
		return e
	}
	if ref.Kind == "manifest_digest" {
		_, e := ParseDigest(ref.ID)
		return e
	}
	if !tagPattern.MatchString(ref.ID) {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func knownMedia(v ManifestMediaType) bool {
	return v == OCIManifest || v == OCIIndex || v == DockerManifest || v == DockerManifestList
}
func header(h http.Header, key string, limit uint64) (string, error) {
	v := h.Values(key)
	if len(v) > 1 {
		return "", types.NewError(types.ProtocolError)
	}
	if len(v) == 0 {
		return "", nil
	}
	if uint64(len(v[0])) > limit {
		return "", types.NewError(types.ResponseLimit)
	}
	return strings.TrimSpace(v[0]), nil
}
func contentInfo(h http.Header, ref types.ResourceRef, limit uint64) (BlobInfo, error) {
	out := BlobInfo{Address: ref}
	raw, e := header(h, "Content-Type", limit)
	if e != nil {
		return out, e
	}
	if raw != "" {
		media, _, e := mime.ParseMediaType(raw)
		if e != nil || !strings.Contains(media, "/") {
			return out, types.NewError(types.ProtocolError)
		}
		out.MediaType = &media
	}
	raw, e = header(h, "Docker-Content-Digest", limit)
	if e != nil {
		return out, e
	}
	if raw != "" {
		v, e := ParseDigest(raw)
		if e != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Digest = &v
		if ref.Kind == "manifest_digest" || ref.Kind == "blob" {
			expected, _ := ParseDigest(ref.ID)
			if expected.Algorithm == v.Algorithm && expected != v {
				return out, types.NewError(types.ScopeMismatch)
			}
		}
	}
	raw, e = header(h, "Content-Length", limit)
	if e != nil {
		return out, e
	}
	if raw != "" {
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				return out, types.NewError(types.ProtocolError)
			}
		}
		n, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return out, types.NewError(types.ProtocolError)
		}
		v, _ := types.ParseInteger(n.String())
		out.Size = &v
	}
	return out, nil
}
func (c *Manifests) Head(ctx context.Context, ref types.ResourceRef, in ManifestReadOptions, o types.CallOptions) (types.Result[ManifestInfo], error) {
	op, bind := c.client.operation("registry_data.oci_v1.manifests.head")
	accept := []string{}
	if len(in.Accept) == 0 {
		in.Accept = []ManifestMediaType{OCIManifest, OCIIndex, DockerManifest, DockerManifestList}
	}
	var invalid error
	for _, v := range in.Accept {
		if !knownMedia(v) {
			invalid = types.NewError(types.InvalidArgument)
		}
		accept = append(accept, string(v))
	}
	op.Headers = http.Header{"Accept": {strings.Join(accept, ",")}}
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: manifestPath(ref), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		if invalid != nil {
			return invalid
		}
		return c.valid(ref)
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ManifestInfo], error) {
		v, e := contentInfo(h, ref, c.client.runtime.StructuredLimit())
		out := ManifestInfo{Address: v.Address, Digest: v.Digest, Size: v.Size, DigestVerified: false}
		if v.MediaType != nil {
			media := ManifestMediaType(*v.MediaType)
			if !knownMedia(media) {
				return core.Decoded[ManifestInfo]{Data: out}, types.NewError(types.ProtocolError)
			}
			out.MediaType = &media
		}
		return core.Decoded[ManifestInfo]{Data: out}, e
	})
}
func (c *Manifests) Put(ctx context.Context, ref types.ResourceRef, in ManifestWriteOptions, o types.CallOptions) (types.Result[ManifestWriteResult], error) {
	op, bind := c.client.operation("registry_data.oci_v1.manifests.replace")
	raw := in.Payload.Reveal()
	op.Headers = http.Header{"Content-Type": {string(in.MediaType)}}
	size, _ := types.ParseInteger(strconv.Itoa(len(raw)))
	var requestedSubject *Digest
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: manifestPath(ref), Body: raw, Transfer: true, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		if e := c.valid(ref); e != nil {
			return e
		}
		if !knownMedia(in.MediaType) || uint64(len(raw)) > c.client.runtime.StructuredLimit() {
			return types.NewError(types.InvalidArgument)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return types.NewError(types.InvalidArgument)
		}
		if value, ok := fields["mediaType"]; ok {
			var media string
			if json.Unmarshal(value, &media) != nil || media != string(in.MediaType) {
				return types.NewError(types.InvalidArgument)
			}
		}
		var subjectError error
		requestedSubject, subjectError = manifestSubject(raw, in.MediaType, *ref.Parent)
		if subjectError != nil {
			return subjectError
		}
		if ref.Kind == "manifest_digest" {
			d, _ := ParseDigest(ref.ID)
			actual, _ := hashBytes(raw, d.Algorithm)
			if d != actual {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, func(_ context.Context, _ io.Reader, ep types.EndpointBinding, _ int, h http.Header) (core.Decoded[ManifestWriteResult], error) {
		digest, _ := hashBytes(raw, "sha256")
		out := ManifestWriteResult{Address: ref, Digest: digest, Size: size, RequestedSubject: requestedSubject}
		if requestedSubject != nil {
			ack, e := header(h, "OCI-Subject", c.client.runtime.StructuredLimit())
			if e != nil {
				return core.Decoded[ManifestWriteResult]{Data: out}, e
			}
			if ack == "" {
				out.NeedsTagFallback = true
			} else {
				parsed, e := ParseDigest(ack)
				if e != nil {
					return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ProtocolError)
				}
				if parsed != *requestedSubject {
					return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ScopeMismatch)
				}
				out.AcknowledgedSubject = &parsed
			}
		}
		stated, e := header(h, "Docker-Content-Digest", c.client.runtime.StructuredLimit())
		if e != nil {
			return core.Decoded[ManifestWriteResult]{Data: out}, e
		}
		if stated != "" {
			d, e := ParseDigest(stated)
			if e != nil {
				return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ProtocolError)
			}
			actual, _ := hashBytes(raw, d.Algorithm)
			if actual != d {
				return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ProtocolError)
			}
			out.Digest = d
		}
		location, e := header(h, "Location", c.client.runtime.StructuredLimit())
		if e != nil {
			return core.Decoded[ManifestWriteResult]{Data: out}, e
		}
		out.Location = types.Protect([]byte(location))
		base, _ := url.Parse(ep.URL)
		parsed, e := url.Parse(location)
		if e != nil || location == "" || parsed.User != nil || parsed.Fragment != "" {
			return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ProtocolError)
		}
		requestURL, _ := url.Parse(strings.TrimRight(ref.EndpointURL, "/") + manifestPath(ref))
		target := requestURL.ResolveReference(parsed)
		if !sameOrigin(base, target) {
			return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ProtocolError)
		}
		prefix := strings.TrimRight(base.Path, "/") + "/v2/" + ref.Parent.Parent.ID + "/" + ref.Parent.ID + "/manifests/"
		if !strings.HasPrefix(target.Path, prefix) {
			return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ScopeMismatch)
		}
		address := strings.TrimPrefix(target.Path, prefix)
		if address != ref.ID {
			d, e := ParseDigest(address)
			if e != nil {
				return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ScopeMismatch)
			}
			actual, _ := hashBytes(raw, d.Algorithm)
			if actual != d {
				return core.Decoded[ManifestWriteResult]{Data: out}, types.NewError(types.ScopeMismatch)
			}
		}
		return core.Decoded[ManifestWriteResult]{Data: out}, nil
	})
}
func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() == "" {
			return "443"
		}
		return u.Port()
	}
	return b.Scheme == "https" && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}
func (c *Manifests) remove(ctx context.Context, ref types.ResourceRef, kind string, o types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.client.operation("registry_data.oci_v1.manifests.delete")
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: manifestPath(ref), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		if ref.Kind != kind {
			return types.NewError(types.InvalidArgument)
		}
		return c.valid(ref)
	}}, func(context.Context, io.Reader, types.EndpointBinding, int, http.Header) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *Manifests) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return c.remove(ctx, ref, "manifest_digest", o)
}
func (c *Manifests) DeleteTag(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return c.remove(ctx, ref, "manifest_tag", o)
}

func manifestSubject(raw []byte, media ManifestMediaType, repo types.ResourceRef) (*Digest, error) {
	if media != OCIManifest && media != OCIIndex {
		return nil, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	value, present := fields["subject"]
	if !present {
		return nil, nil
	}
	var identity struct {
		Digest *string `json:"digest"`
	}
	if json.Unmarshal(value, &identity) != nil || identity.Digest == nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if _, e := ParseDigest(*identity.Digest); types.ErrorCode(e) == types.CapabilityUnavailable {
		return nil, e
	}
	decoded, e := decodeReferrer(value, repo)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return &decoded.Digest, nil
}
