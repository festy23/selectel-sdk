package registrydata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Referrers struct{ client *Client }

func (c *Referrers) List(subject types.ResourceRef, options ReferrerOptions, bounds pagination.Options) (*pagination.Iterator[Referrer], error) {
	if e := c.client.Manifests.valid(subject); e != nil {
		return nil, e
	}
	if subject.Kind != "manifest_digest" {
		return nil, types.NewError(types.InvalidArgument)
	}
	subject = child(*subject.Parent, "manifest_digest", subject.ID)
	if options.ArtifactType != nil {
		v := *options.ArtifactType
		options.ArtifactType = &v
		if strings.ContainsAny(v, "\r\n\x00") || uint64(len(v)) > c.client.runtime.StructuredLimit() {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	op, e := c.client.operation("registry_data.oci_v1.referrers.list")
	if e != nil {
		return nil, e
	}
	op.Headers = http.Header{"Accept": {string(OCIIndex)}}
	plan, e := c.client.runtime.PagePlan(op.ID, "oci_v1", c.client.scope, subject.EndpointID, struct {
		Subject types.ResourceRef
		Options ReferrerOptions
	}{subject, options})
	if e != nil {
		return nil, e
	}
	path := repositoryPath(*subject.Parent) + "/referrers/" + url.PathEscape(subject.ID)
	return pagination.New(plan, bounds, func(ctx context.Context, wire string, _ uint) (pagination.SourcePage[Referrer], error) {
		if uint64(len(wire)) > c.client.runtime.StructuredLimit() {
			return pagination.SourcePage[Referrer]{}, types.NewError(types.ResponseLimit)
		}
		if wire != "" {
			if e := referrerQuery(wire, options); e != nil {
				return pagination.SourcePage[Referrer]{}, e
			}
		}
		query := url.Values{}
		if options.ArtifactType != nil {
			query.Set("artifactType", *options.ArtifactType)
		}
		if wire != "" {
			query = nil
		}
		var next *types.ProtectedValue
		prefix := []Referrer{}
		provider := pagination.ProviderMetadata{}
		result, e := core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, Query: query, OCISessionQuery: wire, Refs: []types.ResourceRef{subject}, PinnedEndpoint: pin(subject)}, func(_ context.Context, reader io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[[]Referrer], error) {
			decoded := func(e error) (core.Decoded[[]Referrer], error) { return core.Decoded[[]Referrer]{Data: prefix}, e }
			content, e := header(h, "Content-Type", c.client.runtime.StructuredLimit())
			if e != nil {
				return decoded(e)
			}
			media, _, e := mime.ParseMediaType(content)
			if e != nil || media != string(OCIIndex) {
				return decoded(types.NewError(types.ProtocolError))
			}
			raw, e := io.ReadAll(io.LimitReader(reader, int64(c.client.runtime.StructuredLimit())+1))
			if e != nil {
				return decoded(types.NewError(types.TransportError))
			}
			if uint64(len(raw)) > c.client.runtime.StructuredLimit() {
				return decoded(types.NewError(types.ResponseLimit))
			}
			var env struct {
				Version   types.Integer     `json:"schemaVersion"`
				MediaType *string           `json:"mediaType"`
				Manifests []json.RawMessage `json:"manifests"`
			}
			if json.Unmarshal(raw, &env) != nil || env.Version.String() != "2" || env.Manifests == nil || (env.MediaType != nil && *env.MediaType != string(OCIIndex)) {
				return decoded(types.NewError(types.ProtocolError))
			}
			var index types.ProtectedJSON
			_ = json.Unmarshal(raw, &index)
			provider.OCIReferrersIndex = &index
			filters, e := referrerFilterHeader(h, c.client.runtime.StructuredLimit())
			if e != nil {
				return decoded(e)
			}
			applied := false
			if filters != "" {
				p := types.Protect([]byte(filters))
				provider.OCIFiltersApplied = &p
				for _, v := range strings.Split(filters, ",") {
					if strings.TrimSpace(v) == "artifactType" {
						applied = true
					}
				}
			}
			for _, row := range env.Manifests {
				v, e := decodeReferrer(row, *subject.Parent)
				if e != nil {
					return decoded(e)
				}
				if options.ArtifactType != nil {
					value := ""
					if v.ArtifactType != nil {
						value = *v.ArtifactType
					}
					if value != *options.ArtifactType {
						if applied {
							return decoded(types.NewError(types.ProtocolError))
						}
						continue
					}
				}
				prefix = append(prefix, v)
			}
			next, e = referrerNext(h, subject, path, wire, options, c.client.runtime.StructuredLimit())
			if e != nil {
				return decoded(e)
			}
			return decoded(nil)
		})
		return pagination.SourcePage[Referrer]{Items: prefix, SensitiveNext: next, Done: e == nil && next == nil, Metadata: result.Metadata, Provider: provider}, e
	})
}
func decodeReferrer(raw []byte, repo types.ResourceRef) (Referrer, error) {
	fields, e := nonnullObject(raw, []string{"mediaType", "size", "digest", "urls", "data", "artifactType", "annotations", "platform"})
	if e != nil {
		return Referrer{}, e
	}
	var wire referrerWire
	if json.Unmarshal(raw, &wire) != nil || wire.Digest == nil || wire.Size == nil || wire.MediaType == nil {
		return Referrer{}, types.NewError(types.ProtocolError)
	}
	digest, e := ParseDigest(*wire.Digest)
	if e != nil {
		return Referrer{}, types.NewError(types.ProtocolError)
	}
	size, e := strconv.ParseInt(wire.Size.String(), 10, 64)
	if e != nil || size < 0 {
		return Referrer{}, types.NewError(types.ProtocolError)
	}
	if _, _, e := mime.ParseMediaType(*wire.MediaType); e != nil {
		return Referrer{}, types.NewError(types.ProtocolError)
	}
	if wire.Platform != nil && (wire.Platform.Architecture == nil || wire.Platform.OS == nil) {
		return Referrer{}, types.NewError(types.ProtocolError)
	}
	if wire.Annotations != nil {
		var annotations map[string]*string
		if json.Unmarshal(wire.Annotations.Reveal(), &annotations) != nil || annotations == nil {
			return Referrer{}, types.NewError(types.ProtocolError)
		}
		for _, v := range annotations {
			if v == nil {
				return Referrer{}, types.NewError(types.ProtocolError)
			}
		}
	}
	if value, ok := fields["urls"]; ok {
		if e := stringArray(value); e != nil {
			return Referrer{}, e
		}
	}
	if value, ok := fields["platform"]; ok {
		platform, e := nonnullObject(value, []string{"architecture", "os", "os.version", "os.features", "variant"})
		if e != nil {
			return Referrer{}, e
		}
		if value, ok := platform["os.features"]; ok {
			if e := stringArray(value); e != nil {
				return Referrer{}, e
			}
		}
	}
	if wire.Data != nil {
		if _, e := base64.StdEncoding.Strict().DecodeString(string(wire.Data.Reveal())); e != nil {
			return Referrer{}, types.NewError(types.ProtocolError)
		}
	}

	return Referrer{Address: child(repo, "manifest_digest", digest.String()), Digest: digest, Size: *wire.Size, MediaType: *wire.MediaType, ArtifactType: wire.ArtifactType, Annotations: wire.Annotations, URLs: wire.URLs, Data: wire.Data, Platform: wire.Platform}, nil
}

func referrerQuery(raw string, options ReferrerOptions) error {
	query, e := url.ParseQuery(raw)
	if e != nil || strings.ContainsAny(raw, "#\r\n\x00") {
		return types.NewError(types.ProtocolError)
	}
	for key := range query {
		switch strings.ToLower(key) {
		case "scope", "service", "authorization", "access_token", "token", "x-auth-token", "x-token", "redirect", "redirect_uri", "mount", "from", "digest", "project", "project_id", "account", "endpoint":
			return types.NewError(types.ScopeMismatch)
		}
	}
	if query.Has("artifactType") && (options.ArtifactType == nil || len(query["artifactType"]) != 1 || query.Get("artifactType") != *options.ArtifactType) {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func referrerNext(h http.Header, subject types.ResourceRef, path, input string, options ReferrerOptions, limit uint64) (*types.ProtectedValue, error) {
	var length uint64
	values := h.Values("Link")
	for _, value := range values {
		length += uint64(len(value))
		if strings.ContainsAny(value, "\r\n\x00") {
			return nil, types.NewError(types.ProtocolError)
		}
		if length > limit {
			return nil, types.NewError(types.ResponseLimit)
		}
	}
	links, e := splitReferrerLinks(strings.Join(values, ","))
	if e != nil {
		return nil, e
	}
	var next *types.ProtectedValue
	for _, link := range links {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		end := strings.IndexByte(link, '>')
		if !strings.HasPrefix(link, "<") || end < 1 {
			return nil, types.NewError(types.ProtocolError)
		}
		_, params, e := mime.ParseMediaType("application/octet-stream" + link[end+1:])
		if e != nil {
			return nil, types.NewError(types.ProtocolError)
		}
		follows := false
		for _, rel := range strings.Fields(params["rel"]) {
			if strings.EqualFold(rel, "next") {
				follows = true
			}
		}
		if !follows {
			continue
		}
		if next != nil || params["anchor"] != "" {
			return nil, types.NewError(types.ProtocolError)
		}
		target, e := url.Parse(link[1:end])
		if e != nil || target.User != nil || target.Fragment != "" || target.Opaque != "" {
			return nil, types.NewError(types.ProtocolError)
		}
		base, _ := url.Parse(subject.EndpointURL)
		requestURL, _ := url.Parse(strings.TrimRight(subject.EndpointURL, "/") + path)
		if input != "" {
			requestURL.RawQuery = input
		} else if options.ArtifactType != nil {
			requestURL.RawQuery = url.Values{"artifactType": {*options.ArtifactType}}.Encode()
		}
		target = requestURL.ResolveReference(target)
		if !sameOrigin(base, target) || target.EscapedPath() != strings.TrimRight(base.EscapedPath(), "/")+path {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if e := referrerQuery(target.RawQuery, options); e != nil {
			return nil, e
		}
		if target.RawQuery == "" {
			return nil, types.NewError(types.ProtocolError)
		}
		value := types.Protect([]byte(target.RawQuery))
		next = &value
	}
	return next, nil
}
func splitReferrerLinks(value string) ([]string, error) {
	var out []string
	start := 0
	angle, quote, escape := false, false, false
	for i, r := range value {
		if escape {
			escape = false
			continue
		}
		if quote {
			if r == '\\' {
				escape = true
			} else if r == '"' {
				quote = false
			}
			continue
		}
		switch r {
		case '"':
			if !angle {
				quote = true
			}
		case '<':
			if angle {
				return nil, types.NewError(types.ProtocolError)
			}
			angle = true
		case '>':
			if !angle {
				return nil, types.NewError(types.ProtocolError)
			}
			angle = false
		case ',':
			if !angle {
				out = append(out, value[start:i])
				start = i + 1
			}
		}
	}
	if angle || quote || escape {
		return nil, types.NewError(types.ProtocolError)
	}
	out = append(out, value[start:])
	return out, nil
}

func nonnullObject(raw []byte, known []string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	for _, name := range known {
		if value, ok := fields[name]; ok && strings.TrimSpace(string(value)) == "null" {
			return nil, types.NewError(types.ProtocolError)
		}
	}
	return fields, nil
}
func stringArray(raw []byte) error {
	var values []*string
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return types.NewError(types.ProtocolError)
	}
	for _, v := range values {
		if v == nil {
			return types.NewError(types.ProtocolError)
		}
	}
	return nil
}

func referrerFilterHeader(h http.Header, limit uint64) (string, error) {
	values := h.Values("OCI-Filters-Applied")
	var length uint64
	for _, v := range values {
		length += uint64(len(v)) + 1
		if length > limit {
			return "", types.NewError(types.ResponseLimit)
		}
		if strings.ContainsAny(v, "\r\n\x00") {
			return "", types.NewError(types.ProtocolError)
		}
	}
	return strings.Join(values, ","), nil
}
