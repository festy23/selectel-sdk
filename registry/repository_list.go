package registry

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type RepositoryListOptions struct {
	Search    *string        `json:"search,omitempty"`
	SortField *string        `json:"sort_field,omitempty"`
	SortType  *string        `json:"sort_type,omitempty"`
	Short     *bool          `json:"short,omitempty"`
	Offset    *types.Integer `json:"offset,omitempty"`
	// NativeEmpty204 selects the pinned public Go SDK's empty-list status variant.
	NativeEmpty204 bool `json:"native_empty_204,omitempty"`
}

func repositoryFilters(f RepositoryListOptions) (url.Values, string, error) {
	if (f.Search != nil && !utf8.ValidString(*f.Search)) || (f.SortField != nil && *f.SortField != "name" && *f.SortField != "updatedAt" && *f.SortField != "size") || (f.SortType != nil && *f.SortType != "asc" && *f.SortType != "desc") || (f.Short != nil && *f.Short && f.SortField != nil && *f.SortField != "name") {
		return nil, "", types.NewError(types.InvalidArgument)
	}
	q := url.Values{}
	if f.Search != nil {
		q.Set("search", *f.Search)
	}
	if f.SortField != nil {
		q.Set("sort_field", *f.SortField)
	}
	if f.SortType != nil {
		q.Set("sort_type", *f.SortType)
	}
	if f.Short != nil {
		q.Set("short", strconv.FormatBool(*f.Short))
	}
	initial := "0"
	if f.Offset != nil {
		n, ok := new(big.Int).SetString(f.Offset.String(), 10)
		if !ok || n.Sign() < 0 {
			return nil, "", types.NewError(types.InvalidArgument)
		}
		initial = n.String()
	}
	return q, initial, nil
}
func offsetManagementList[T any](r *core.Runtime, scope types.Scope, id string, ref types.ResourceRef, path string, q url.Values, initial string, native204 bool, o pagination.Options, decode func([]byte) ([]T, *types.Integer, *types.Integer, error)) (*pagination.Iterator[T], error) {
	op, e := r.BindDocumented(managementOperations[id], scope)
	if e != nil {
		return nil, e
	}
	if e = managementValid(ref, scope, op.APIGeneration, managementRefKinds[id]); e != nil {
		return nil, e
	}
	ref = snapshotRegistryRef(ref)
	if native204 {
		if id != "registry.v1.registries_repositories.list" {
			return nil, types.NewError(types.InvalidArgument)
		}
		op.Success = append(op.Success, 204)
		op.NoContentSuccess = append(op.NoContentSuccess, 204)
	}
	semantic := struct {
		Parent    types.ResourceRef
		Query     url.Values
		Offset    string
		Native204 bool
	}{ref, q, initial, native204}
	plan, e := r.PagePlan(id, op.APIGeneration, scope, op.EndpointID, semantic)
	if e != nil {
		return nil, e
	}
	return pagination.New(plan, o, func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		if cursor == "" {
			cursor = initial
		}
		offset, ok := new(big.Int).SetString(cursor, 10)
		if !ok || offset.Sign() < 0 {
			return pagination.SourcePage[T]{}, types.NewError(types.InvalidArgument)
		}
		query := url.Values{}
		for k, values := range q {
			query[k] = append([]string(nil), values...)
		}
		query.Set("offset", offset.String())
		query.Set("limit", strconv.FormatUint(uint64(size), 10))
		provider := pagination.ProviderMetadata{}
		done := false
		next := ""
		var prefix []T
		result, e := core.DoJSON(r, ctx, op, core.Request{Scope: scope, Path: path, Query: query, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref)}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[[]T], error) {
			items, total, filtered, e := decode(raw)
			prefix = items
			if e != nil {
				return core.Decoded[[]T]{}, e
			}
			provider.TotalCount = total
			provider.ReportedCount = filtered
			end := new(big.Int).Add(offset, big.NewInt(int64(len(items))))
			done = len(items) == 0
			if filtered != nil {
				n, ok := new(big.Int).SetString(filtered.String(), 10)
				if !ok || n.Sign() < 0 {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				if total == nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				all, ok := new(big.Int).SetString(total.String(), 10)
				if !ok || all.Sign() < 0 || n.Cmp(all) > 0 || (len(items) > 0 && end.Cmp(n) > 0) {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				if len(items) == 0 && offset.Cmp(n) < 0 {
					return core.Decoded[[]T]{}, types.NewError(types.Incomplete)
				}
				done = end.Cmp(n) >= 0
			}
			if !done {
				next = end.String()
			}
			return core.Decoded[[]T]{Data: items}, nil
		})
		if e != nil {
			result.Data = prefix
		}
		return pagination.SourcePage[T]{Items: result.Data, Metadata: result.Metadata, Provider: provider, Done: done, Next: next}, e
	})
}
func (c *V1Repositories) Iterate(ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (*pagination.Iterator[RepositoryInfo], error) {
	ref = snapshotRegistryRef(ref)
	q, initial, e := repositoryFilters(f)
	if e != nil {
		return nil, e
	}
	return offsetManagementList(c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.list", ref, registryPath(ref.ID)+"/repositories", q, initial, f.NativeEmpty204, o, func(raw []byte) ([]RepositoryInfo, *types.Integer, *types.Integer, error) {
		if len(raw) == 0 {
			if !f.NativeEmpty204 {
				return nil, nil, nil, types.NewError(types.ProtocolError)
			}
			return []RepositoryInfo{}, nil, nil, nil
		}
		items, e := registryRows(raw, func(v *RepositoryInfo) error {
			if v.Name != nil {
				child, e := nestedRef(ref, "repository", *v.Name)
				if e != nil {
					return e
				}
				v.Ref = &child
			}
			return nil
		})
		return items, nil, nil, e
	})
}
func (c *V1Repositories) ListPage(ctx context.Context, ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (pagination.Page[RepositoryInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Page[RepositoryInfo]{}, e
	}
	return it.Next(ctx)
}
func (c *V1Repositories) Collect(ctx context.Context, ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (pagination.Collection[RepositoryInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Collection[RepositoryInfo]{Items: []RepositoryInfo{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
