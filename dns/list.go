package dns

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

type ZoneListOptions struct {
	Filter *string
	SortBy []ZonesSortingOptions
	Offset *types.Integer
}

func (c *Zones) Iterate(f ZoneListOptions, o pagination.Options) (*pagination.Iterator[Zone], error) {
	q := url.Values{}
	if f.Filter != nil {
		q.Set("filter", *f.Filter)
	}
	for _, v := range f.SortBy {
		switch v {
		case "name.ascend", "name.descend", "created_at.ascend", "created_at.descend", "updated_at.ascend", "updated_at.descend":
			q.Add("sort_by", string(v))
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	return iterate[Zone](c.service, "zones", nil, q, f.Offset, o, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Zone], error) { return decodeZone(raw, ep, "") })
}
func (c *Zones) ListPage(ctx context.Context, f ZoneListOptions, o pagination.Options) (pagination.Page[Zone], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Zone]{}, e
	}
	return it.Next(ctx)
}
func (c *Zones) Collect(ctx context.Context, f ZoneListOptions, o pagination.Options) (pagination.Collection[Zone], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Zone]{Complete: false}, e
	}
	return it.Collect(ctx)
}

type providerPage[T any] struct {
	Items []T
	Next  string
	Count *types.Integer
}

func nonnegative(raw string) (*big.Int, bool) {
	v, ok := new(big.Int).SetString(raw, 10)
	return v, ok && v.Sign() >= 0
}
func iterate[T any](s service, resource string, parent *types.ResourceRef, q url.Values, initial *types.Integer, o pagination.Options, decode core.Decoder[T]) (*pagination.Iterator[T], error) {
	if e := s.scopeValid(); e != nil {
		return nil, e
	}
	initialOffset := "0"
	if initial != nil {
		if _, ok := nonnegative(initial.String()); !ok {
			return nil, types.NewError(types.InvalidArgument)
		}
		initialOffset = initial.String()
	}
	operation, e := s.runtime.BindDocumented(op(resource, "list", "GET", 200), s.scope)
	if e != nil {
		return nil, e
	}
	binding := types.EndpointBinding{ID: operation.EndpointID, URL: operation.FixedEndpoint, Scope: s.scope, APIGeneration: "v2", Audience: "selectel"}
	pinned := &binding
	path := "/zones"
	refs := []types.ResourceRef{}
	if parent != nil {
		if e := s.valid(*parent, "zone"); e != nil {
			return nil, e
		}
		pinned = pin(*parent)
		path += "/" + segment(parent.ID) + "/rrset"
		refs = append(refs, *parent)
	}
	digest := sha256.Sum256([]byte(q.Encode() + "\n" + path + "\n" + binding.URL + "\n" + initialOffset))
	plan, e := s.runtime.PagePlan(operation.ID, "v2", s.scope, operation.EndpointID, hex.EncodeToString(digest[:]))
	if e != nil {
		return nil, e
	}
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		offset := cursor
		if offset == "" {
			offset = initialOffset
		}
		current, ok := nonnegative(offset)
		if !ok {
			return pagination.SourcePage[T]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{}
		for k, v := range q {
			query[k] = append([]string(nil), v...)
		}
		query.Set("offset", offset)
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		result, e := core.DoJSON(s.runtime, ctx, operation, core.Request{Scope: s.scope, Path: path, Query: query, Refs: refs, PinnedEndpoint: pinned}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[providerPage[T]], error) {
			var env map[string]json.RawMessage
			var count *types.Integer
			var next *types.Integer
			var rows []json.RawMessage
			if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env["count"], &count) != nil || count == nil || json.Unmarshal(env["next_offset"], &next) != nil || env["next_offset"] == nil || json.Unmarshal(env["result"], &rows) != nil || rows == nil {
				return core.Decoded[providerPage[T]]{}, types.NewError(types.ProtocolError)
			}
			if _, ok := nonnegative(count.String()); !ok {
				return core.Decoded[providerPage[T]]{}, types.NewError(types.ProtocolError)
			}
			data := providerPage[T]{Items: make([]T, 0, len(rows)), Count: count}
			if next != nil {
				n, ok := nonnegative(next.String())
				if !ok || n.Sign() != 0 && n.Cmp(current) <= 0 {
					return core.Decoded[providerPage[T]]{}, types.NewError(types.ProtocolError)
				}
				if n.Sign() != 0 {
					data.Next = next.String()
				}
			}
			for _, row := range rows {
				v, e := decode(row, ep)
				if e != nil {
					return core.Decoded[providerPage[T]]{}, e
				}
				data.Items = append(data.Items, v.Data)
			}
			return core.Decoded[providerPage[T]]{Data: data}, nil
		})
		return pagination.SourcePage[T]{Items: result.Data.Items, Next: result.Data.Next, Done: result.Data.Next == "", Metadata: result.Metadata, Provider: pagination.ProviderMetadata{ReportedCount: result.Data.Count}}, e
	}
	return pagination.New(plan, o, fetch)
}

// Search is protected because current DNS API searches record content as well
// as names. Its private digest binds cursors without disclosing the value.
type RRSetListOptions struct {
	SortBy     []RRSetSortingOptions
	RRSetTypes []string
	Name       *string
	Search     *types.ProtectedValue
	ManagedBy  *string
	Offset     *types.Integer
}

func (c *RRSets) Iterate(parent types.ResourceRef, f RRSetListOptions, o pagination.Options) (*pagination.Iterator[RRSet], error) {
	q := url.Values{}
	if f.Name != nil {
		q.Set("name", *f.Name)
	}
	if f.Search != nil {
		q.Set("search", string(f.Search.Reveal()))
	}
	if f.ManagedBy != nil {
		if !uuidPattern.MatchString(*f.ManagedBy) {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Set("managed_by", *f.ManagedBy)
	}
	for _, v := range f.RRSetTypes {
		q.Add("rrset_types", v)
	}
	for _, v := range f.SortBy {
		switch v {
		case "type.ascend", "type.descend", "name.ascend", "name.descend":
			q.Add("sort_by", string(v))
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	return iterate[RRSet](c.service, "rrsets", &parent, q, f.Offset, o, func(raw []byte, ep types.EndpointBinding) (core.Decoded[RRSet], error) {
		return decodeRR(raw, ep, parent, "")
	})
}
func (c *RRSets) ListPage(ctx context.Context, parent types.ResourceRef, f RRSetListOptions, o pagination.Options) (pagination.Page[RRSet], error) {
	it, e := c.Iterate(parent, f, o)
	if e != nil {
		return pagination.Page[RRSet]{}, e
	}
	return it.Next(ctx)
}
func (c *RRSets) Collect(ctx context.Context, parent types.ResourceRef, f RRSetListOptions, o pagination.Options) (pagination.Collection[RRSet], error) {
	it, e := c.Iterate(parent, f, o)
	if e != nil {
		return pagination.Collection[RRSet]{Complete: false}, e
	}
	return it.Collect(ctx)
}
