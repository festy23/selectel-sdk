package ptr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
)

type ListOptions struct {
	Offset *types.Integer
	Filter *types.ProtectedValue
	types.CallOptions
}

// Count cannot decode a source-confirmed count response: OpenAPI describes an array even for only-count=true.
func (c *Client) Count(context.Context, ListOptions, types.CallOptions) (types.Result[types.Integer], error) {
	return types.Result[types.Integer]{}, types.NewError(types.CapabilityUnavailable)
}
func (c *Client) Iterate(in ListOptions, options pagination.Options) (*pagination.Iterator[Record], error) {
	if e := c.validScope(); e != nil {
		return nil, e
	}
	offset := "0"
	if in.Offset != nil {
		v, ok := new(big.Int).SetString(in.Offset.String(), 10)
		if !ok || v.Sign() < 0 {
			return nil, types.NewError(types.InvalidArgument)
		}
		offset = v.String()
	}
	operation, e := c.runtime.BindDocumented(op("list", "GET"), c.scope)
	if e != nil {
		return nil, e
	}
	q := url.Values{}
	if in.Filter != nil {
		q.Set("filter", string(in.Filter.Reveal()))
	}
	digest := sha256.Sum256([]byte(q.Encode()))
	plan, e := c.runtime.PagePlan(operation.ID, "v1", c.scope, operation.EndpointID, struct{ URL, Offset, Digest string }{operation.FixedEndpoint, offset, hex.EncodeToString(digest[:])})
	if e != nil {
		return nil, e
	}
	return pagination.New(plan, options, func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[Record], error) {
		start := offset
		if cursor != "" {
			start = cursor
		}
		n, ok := new(big.Int).SetString(start, 10)
		if !ok || n.Sign() < 0 {
			return pagination.SourcePage[Record]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{}
		for k, v := range q {
			query[k] = append([]string(nil), v...)
		}
		query.Set("offset", n.String())
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		v, e := core.DoJSON(c.runtime, ctx, operation, core.Request{Scope: c.scope, Path: "/", Query: query, Options: in.CallOptions, Validate: c.validScope}, func(raw []byte, ep types.EndpointBinding) (d core.Decoded[[]Record], e error) {
			var rows []json.RawMessage
			if json.Unmarshal(raw, &rows) != nil || rows == nil {
				return d, types.NewError(types.ProtocolError)
			}
			for _, row := range rows {
				item, e := decoder("")(row, ep)
				if e != nil {
					return d, e
				}
				d.Data = append(d.Data, item.Data)
			}
			return d, nil
		})
		next := new(big.Int).Add(n, big.NewInt(int64(len(v.Data)))).String()
		return pagination.SourcePage[Record]{Items: v.Data, Next: next, Done: len(v.Data) == 0, Metadata: v.Metadata}, e
	})
}
func (c *Client) ListPage(ctx context.Context, in ListOptions, o pagination.Options) (pagination.Page[Record], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Page[Record]{}, e
	}
	return it.Next(ctx)
}
func (c *Client) Collect(ctx context.Context, in ListOptions, o pagination.Options) (pagination.Collection[Record], error) {
	it, e := c.Iterate(in, o)
	if e != nil {
		return pagination.Collection[Record]{}, e
	}
	return it.Collect(ctx)
}
