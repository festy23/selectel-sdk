package registrydata

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (c *Manifests) Get(ctx context.Context, ref types.ResourceRef, in ManifestReadOptions, o types.CallOptions) (types.Result[ManifestDocument], error) {
	op, bind := c.client.operation("registry_data.oci_v1.manifests.get")
	accept := []string{}
	if len(in.Accept) == 0 {
		in.Accept = []ManifestMediaType{OCIManifest, OCIIndex, DockerManifest, DockerManifestList}
	}
	var invalid error
	for _, v := range in.Accept {
		if v != OCIManifest && v != OCIIndex && v != DockerManifest && v != DockerManifestList {
			invalid = types.NewError(types.InvalidArgument)
		}
		accept = append(accept, string(v))
	}
	op.Headers = http.Header{"Accept": {strings.Join(accept, ",")}}
	path := ""
	if ref.Parent != nil && ref.Parent.Parent != nil {
		path = "/v2/" + ref.Parent.Parent.ID + "/" + ref.Parent.ID + "/manifests/" + url.PathEscape(ref.ID)
	}
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, Transfer: true, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bind != nil {
			return bind
		}
		if invalid != nil {
			return invalid
		}
		if ref.Kind != "manifest_tag" && ref.Kind != "manifest_digest" {
			return types.NewError(types.InvalidArgument)
		}
		if e := c.client.valid(ref, ref.Kind); e != nil {
			return e
		}
		if ref.Kind == "manifest_digest" {
			if _, e := ParseDigest(ref.ID); e != nil {
				return e
			}
		}
		if ref.Kind == "manifest_tag" && !tagPattern.MatchString(ref.ID) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(_ context.Context, body io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ManifestDocument], error) {
		limit := c.client.runtime.StructuredLimit()
		raw, e := io.ReadAll(io.LimitReader(body, int64(limit)+1))
		if e != nil {
			return core.Decoded[ManifestDocument]{}, types.NewError(types.TransportError)
		}
		if uint64(len(raw)) > limit {
			return core.Decoded[ManifestDocument]{}, types.NewError(types.ResponseLimit)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[ManifestDocument]{}, types.NewError(types.ProtocolError)
		}
		rawType, e := header(h, "Content-Type", limit)
		if e != nil {
			return core.Decoded[ManifestDocument]{}, e
		}
		media, _, e := mime.ParseMediaType(rawType)
		if e != nil || (media != string(OCIManifest) && media != string(OCIIndex) && media != string(DockerManifest) && media != string(DockerManifestList)) {
			return core.Decoded[ManifestDocument]{}, types.NewError(types.ProtocolError)
		}
		if value, ok := fields["mediaType"]; ok {
			var declared string
			if json.Unmarshal(value, &declared) != nil || declared != media {
				return core.Decoded[ManifestDocument]{}, types.NewError(types.ProtocolError)
			}
		}
		digest, _ := hashBytes(raw, "sha256")
		stated, e := header(h, "Docker-Content-Digest", limit)
		if e != nil {
			return core.Decoded[ManifestDocument]{}, e
		}
		for _, expected := range []string{stated, func() string {
			if ref.Kind == "manifest_digest" {
				return ref.ID
			}
			return ""
		}()} {
			if expected != "" {
				d, e := ParseDigest(expected)
				if e != nil {
					return core.Decoded[ManifestDocument]{}, types.NewError(types.ProtocolError)
				}
				actual, _ := hashBytes(raw, d.Algorithm)
				if actual != d {
					return core.Decoded[ManifestDocument]{}, types.NewError(types.ProtocolError)
				}
				digest = d
			}
		}
		size, _ := types.ParseInteger(strconv.Itoa(len(raw)))
		var payload types.ProtectedJSON
		_ = json.Unmarshal(raw, &payload)
		return core.Decoded[ManifestDocument]{Data: ManifestDocument{Address: ref, Digest: digest, MediaType: ManifestMediaType(media), Size: size, Payload: payload}}, nil
	})
}
