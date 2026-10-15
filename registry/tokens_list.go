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
	"unicode/utf8"
)

type TokenListOptions struct {
	Search    *string        `json:"search,omitempty"`
	SortField *string        `json:"sort_field,omitempty"`
	SortType  *string        `json:"sort_type,omitempty"`
	ScopeMode *string        `json:"scope_mode,omitempty"`
	Offset    *types.Integer `json:"offset,omitempty"`
}

func (c *V2Tokens) Iterate(f TokenListOptions, o pagination.Options) (*pagination.Iterator[TokenInfo], error) {
	if (f.Search != nil && !utf8.ValidString(*f.Search)) || (f.SortField != nil && *f.SortField != "name" && *f.SortField != "createdAt" && *f.SortField != "lastUsedAt" && *f.SortField != "expiresAt") || (f.SortType != nil && *f.SortType != "asc" && *f.SortType != "desc") || (f.ScopeMode != nil && *f.ScopeMode != "ro" && *f.ScopeMode != "rw") {
		return nil, types.NewError(types.InvalidArgument)
	}
	initial := "0"
	if f.Offset != nil {
		n, ok := new(big.Int).SetString(f.Offset.String(), 10)
		if !ok || n.Sign() < 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		initial = n.String()
	}
	q := url.Values{}
	for key, value := range map[string]*string{"search": f.Search, "sort_field": f.SortField, "sort_type": f.SortType, "scope_mode": f.ScopeMode} {
		if value != nil {
			q.Set(key, *value)
		}
	}
	op, e := c.registries.runtime.BindDocumented(tokenOperations["registry.v2.tokens.list"], c.registries.scope)
	if e != nil {
		return nil, e
	}
	semantic := struct{ Query, Initial, Endpoint string }{q.Encode(), initial, op.FixedEndpoint}
	plan, e := c.registries.runtime.PagePlan(op.ID, "v2", c.registries.scope, op.EndpointID, semantic)
	if e != nil {
		return nil, e
	}
	return pagination.New(plan, o, func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[TokenInfo], error) {
		if cursor == "" {
			cursor = initial
		}
		offset, ok := new(big.Int).SetString(cursor, 10)
		if !ok || offset.Sign() < 0 {
			return pagination.SourcePage[TokenInfo]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{}
		for key, values := range q {
			query[key] = append([]string(nil), values...)
		}
		query.Set("offset", offset.String())
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		provider := pagination.ProviderMetadata{}
		var prefix []TokenInfo
		out, e := core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: "/v2/tokens", Query: query}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]TokenInfo], error) {
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				return core.Decoded[[]TokenInfo]{}, types.NewError(types.ProtocolError)
			}
			var total *types.Integer
			var rows []json.RawMessage
			if json.Unmarshal(fields["totalCount"], &total) != nil || total == nil || json.Unmarshal(fields["tokens"], &rows) != nil || rows == nil {
				return core.Decoded[[]TokenInfo]{}, types.NewError(types.ProtocolError)
			}
			n, ok := new(big.Int).SetString(total.String(), 10)
			if !ok || n.Sign() < 0 {
				return core.Decoded[[]TokenInfo]{}, types.NewError(types.ProtocolError)
			}
			provider.TotalCount = total
			prefix = []TokenInfo{}
			for _, row := range rows {
				v, e := decodeToken("")(row, ep)
				if e != nil {
					return core.Decoded[[]TokenInfo]{}, e
				}
				prefix = append(prefix, v.Data)
			}
			return core.Decoded[[]TokenInfo]{Data: prefix}, nil
		})
		if e != nil {
			out.Data = prefix
		}
		page := pagination.SourcePage[TokenInfo]{Items: out.Data, Provider: provider, Metadata: out.Metadata, Done: e == nil && len(out.Data) == 0}
		if e == nil && !page.Done {
			page.Next = new(big.Int).Add(offset, big.NewInt(int64(len(out.Data)))).String()
		}
		return page, e
	})
}
func (c *V2Tokens) ListPage(ctx context.Context, f TokenListOptions, o pagination.Options) (pagination.Page[TokenInfo], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[TokenInfo]{}, e
	}
	return it.Next(ctx)
}
func (c *V2Tokens) Collect(ctx context.Context, f TokenListOptions, o pagination.Options) (pagination.Collection[TokenInfo], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[TokenInfo]{Items: []TokenInfo{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
