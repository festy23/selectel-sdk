package registrydata

import (
	"context"
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

type Tags struct{ client *Client }

func (c *Client) Version(ctx context.Context, o types.CallOptions) (types.Result[VersionInfo], error) {
	op, bind := c.operation("registry_data.oci_v1.version.get")
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/", Options: o, Validate: func() error { return bind }}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[VersionInfo], error) {
		out := VersionInfo{Supported: true}
		raw, e := header(h, "Docker-Distribution-API-Version", c.runtime.StructuredLimit())
		if raw != "" {
			out.DistributionVersion = &raw
		}
		return core.Decoded[VersionInfo]{Data: out}, e
	})
}
func (c *Tags) List(repo types.ResourceRef, options pagination.Options) (*pagination.Iterator[types.ResourceRef], error) {
	if e := c.client.valid(repo, "repository"); e != nil {
		return nil, e
	}
	// The iterator owns a deep snapshot of all parent identity and endpoint fields.
	repo = child(*repo.Parent, "repository", repo.ID)
	op, e := c.client.operation("registry_data.oci_v1.tags.list")
	if e != nil {
		return nil, e
	}
	plan, e := c.client.runtime.PagePlan(op.ID, "oci_v1", c.client.scope, repo.EndpointID, repo)
	if e != nil {
		return nil, e
	}
	path := repositoryPath(repo) + "/tags/list"
	return pagination.New(plan, options, func(ctx context.Context, last string, size uint) (pagination.SourcePage[types.ResourceRef], error) {
		if last != "" && !tagPattern.MatchString(last) {
			return pagination.SourcePage[types.ResourceRef]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{"n": {strconv.FormatUint(uint64(size), 10)}}
		if last != "" {
			query.Set("last", last)
		}
		next := ""
		done := false
		var prefix []types.ResourceRef
		result, e := core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, Query: query, Refs: []types.ResourceRef{repo}, PinnedEndpoint: pin(repo)}, func(_ context.Context, body io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[[]types.ResourceRef], error) {
			var out []types.ResourceRef
			decoded := func(e error) (core.Decoded[[]types.ResourceRef], error) {
				prefix = append([]types.ResourceRef{}, out...)
				return core.Decoded[[]types.ResourceRef]{Data: out}, e
			}
			raw, e := io.ReadAll(io.LimitReader(body, int64(c.client.runtime.StructuredLimit())+1))
			if e != nil {
				return decoded(types.NewError(types.TransportError))
			}
			if uint64(len(raw)) > c.client.runtime.StructuredLimit() {
				return decoded(types.NewError(types.ResponseLimit))
			}
			var payload struct {
				Name *string   `json:"name"`
				Tags *[]string `json:"tags"`
			}
			if json.Unmarshal(raw, &payload) != nil || payload.Name == nil || payload.Tags == nil {
				return decoded(types.NewError(types.ProtocolError))
			}
			if *payload.Name != repo.Parent.ID+"/"+repo.ID {
				return decoded(types.NewError(types.ScopeMismatch))
			}
			if uint(len(*payload.Tags)) > size {
				return decoded(types.NewError(types.ProtocolError))
			}
			seen := map[string]bool{}
			previous := last
			for _, tag := range *payload.Tags {
				if !tagPattern.MatchString(tag) || seen[tag] || tag == last || strings.ToLower(tag) < strings.ToLower(previous) {
					return decoded(types.NewError(types.ProtocolError))
				}
				seen[tag] = true
				previous = tag
				out = append(out, child(repo, "manifest_tag", tag))
			}
			link, e := header(h, "Link", c.client.runtime.StructuredLimit())
			if e != nil {
				return decoded(e)
			}
			if link != "" {
				// The supported next-link subset carries only the source-defined n/last.
				start, end := strings.Index(link, "<"), strings.Index(link, ">")
				if start != 0 || end < 1 || len(out) == 0 {
					return decoded(types.NewError(types.ProtocolError))
				}
				_, params, e := mime.ParseMediaType("application/octet-stream" + link[end+1:])
				if e != nil || len(params) != 1 || params["rel"] != "next" {
					return decoded(types.NewError(types.ProtocolError))
				}
				u, e := url.Parse(link[1:end])
				if e != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" {
					return decoded(types.NewError(types.ProtocolError))
				}
				base, _ := url.Parse(repo.EndpointURL)
				requestURL, _ := url.Parse(strings.TrimRight(repo.EndpointURL, "/") + path)
				u = requestURL.ResolveReference(u)
				q, e := url.ParseQuery(u.RawQuery)
				if e != nil || !sameOrigin(base, u) || u.EscapedPath() != strings.TrimRight(base.EscapedPath(), "/")+path || len(q) != 2 || len(q["n"]) != 1 || len(q["last"]) != 1 || q.Get("n") != strconv.FormatUint(uint64(size), 10) || q.Get("last") != previous {
					return decoded(types.NewError(types.ScopeMismatch))
				}
				next = previous
			} else if uint(len(out)) == size {
				next = previous
			} else {
				done = true
			}
			return decoded(nil)
		})
		return pagination.SourcePage[types.ResourceRef]{Items: prefix, Next: next, Done: done, Metadata: result.Metadata}, e
	})
}
