// Package registry manages project-scoped Container Registry APIs.
package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
)

type Client struct {
	V1 V1Client
	V2 V2Client
}
type V1Client struct{ Registries *V1Registries }
type V2Client struct{ Registries *Registries }
type Registries struct {
	runtime *core.Runtime
	scope   types.Scope
}
type ListOptions struct {
	Search    *string `json:"search,omitempty"`
	SortField *string `json:"sort_field,omitempty"`
	SortType  *string `json:"sort_type,omitempty"`
}

func Bind(r *core.Runtime, scope types.Scope) *Client {
	return &Client{V1: V1Client{Registries: &V1Registries{runtime: r, scope: scope}}, V2: V2Client{Registries: &Registries{runtime: r, scope: scope}}}
}
func (c *Registries) Iterate(f ListOptions, o pagination.Options) (*pagination.Iterator[Registry], error) {
	if f.SortField != nil && *f.SortField != "name" && *f.SortField != "createdAt" && *f.SortField != "size" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if f.SortType != nil && *f.SortType != "asc" && *f.SortType != "desc" {
		return nil, types.NewError(types.InvalidArgument)
	}
	op, e := c.runtime.BindDocumented(core.Operation{ID: "registry.v2.registries.list", Service: "registry", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}}, c.scope)
	if e != nil {
		return nil, e
	}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, f)
	if e != nil {
		return nil, e
	}
	filters := url.Values{}
	if f.Search != nil {
		filters.Set("search", *f.Search)
	}
	if f.SortField != nil {
		filters.Set("sort_field", *f.SortField)
	}
	if f.SortType != nil {
		filters.Set("sort_type", *f.SortType)
	}
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[Registry], error) {
		offset := new(big.Int)
		if cursor != "" {
			if _, ok := offset.SetString(cursor, 10); !ok || offset.Sign() < 0 {
				return pagination.SourcePage[Registry]{}, types.NewError(types.InvalidArgument)
			}
		}
		q := url.Values{"limit": {strconv.FormatUint(uint64(size), 10)}, "offset": {offset.String()}}
		for key, values := range filters {
			q[key] = append([]string(nil), values...)
		}
		done := false
		next := ""
		provider := pagination.ProviderMetadata{}
		decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]Registry], error) {
			var body struct {
				Registries []Registry     `json:"registries"`
				Total      *types.Integer `json:"totalCount"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Registries == nil || body.Total == nil {
				return core.Decoded[[]Registry]{}, types.NewError(types.ProtocolError)
			}
			total, ok := new(big.Int).SetString(body.Total.String(), 10)
			if !ok || total.Sign() < 0 {
				return core.Decoded[[]Registry]{}, types.NewError(types.ProtocolError)
			}
			for i := range body.Registries {
				v := &body.Registries[i]
				if err := v.Validate(); err != nil {
					return core.Decoded[[]Registry]{}, err
				}
				if v.ID != nil {
					v.Ref = core.Ref(endpoint, "registry", "registry", *v.ID)
				}
			}
			nextOffset := new(big.Int).Add(offset, big.NewInt(int64(len(body.Registries))))
			provider.TotalCount = body.Total
			done = len(body.Registries) == 0 || nextOffset.Cmp(total) >= 0
			if !done {
				next = nextOffset.String()
			}
			return core.Decoded[[]Registry]{Data: body.Registries}, nil
		}
		r, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/registries", Query: q}, decode)
		if err != nil {
			provider = pagination.ProviderMetadata{}
		}
		return pagination.SourcePage[Registry]{Items: r.Data, Next: next, Done: done, Metadata: r.Metadata, Provider: provider}, err
	}
	return pagination.New(plan, o, fetch)
}
func (c *Registries) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[Registry], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Registry]{}, e
	}
	return it.Next(ctx)
}
func (c *Registries) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[Registry], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Registry]{Items: []Registry{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
