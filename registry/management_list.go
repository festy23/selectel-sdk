package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
)

func fixedManagementList[T any](r *core.Runtime, scope types.Scope, id string, ref types.ResourceRef, path string, o pagination.Options, decode func([]byte) ([]T, error)) (*pagination.Iterator[T], error) {
	op, e := r.BindDocumented(managementOperations[id], scope)
	if e != nil {
		return nil, e
	}
	if e = managementValid(ref, scope, op.APIGeneration, managementRefKinds[id]); e != nil {
		return nil, e
	}
	plan, e := r.PagePlan(op.ID, op.APIGeneration, scope, op.EndpointID, ref)
	if e != nil {
		return nil, e
	}
	plan.FixedSize = true
	return pagination.New(plan, o, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[T], error) {
		var prefix []T
		out, e := managementCall(ctx, r, scope, id, ref, path, nil, types.CallOptions{}, func(raw []byte) ([]T, error) { items, err := decode(raw); prefix = items; return items, err })
		if e != nil {
			out.Data = prefix
		}
		return pagination.SourcePage[T]{Items: out.Data, Metadata: out.Metadata, Done: e == nil}, e
	})
}
func (c *V1Images) Iterate(ref types.ResourceRef, o pagination.Options) (*pagination.Iterator[ImageInfo], error) {
	ref = snapshotRegistryRef(ref)
	return fixedManagementList(c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories_images.list", ref, repositoryPath(ref)+"/images", o, func(raw []byte) ([]ImageInfo, error) {
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil || rows == nil {
			return nil, types.NewError(types.ProtocolError)
		}
		out := []ImageInfo{}
		for _, row := range rows {
			var v ImageInfo
			if json.Unmarshal(row, &v) != nil || v.Validate() != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if v.Digest != nil {
				child, e := nestedRef(ref, "manifest", *v.Digest)
				if e != nil {
					return out, types.NewError(types.ProtocolError)
				}
				v.Ref = &child
			}
			out = append(out, v)
		}
		return out, nil
	})
}
func (c *V1Images) ListPage(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Page[ImageInfo], error) {
	it, e := c.Iterate(ref, o)
	if e != nil {
		return pagination.Page[ImageInfo]{}, e
	}
	return it.Next(ctx)
}
func (c *V1Images) Collect(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Collection[ImageInfo], error) {
	it, e := c.Iterate(ref, o)
	if e != nil {
		return pagination.Collection[ImageInfo]{Items: []ImageInfo{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *V1Repositories) IterateTags(ref types.ResourceRef, o pagination.Options) (*pagination.Iterator[string], error) {
	ref = snapshotRegistryRef(ref)
	return fixedManagementList(c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories_tags.list", ref, repositoryPath(ref)+"/tags", o, func(raw []byte) ([]string, error) {
		return registryRows[string](raw, nil)
	})
}
func (c *V1Repositories) TagsPage(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Page[string], error) {
	it, e := c.IterateTags(ref, o)
	if e != nil {
		return pagination.Page[string]{}, e
	}
	return it.Next(ctx)
}
func (c *V1Repositories) Tags(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Collection[string], error) {
	it, e := c.IterateTags(ref, o)
	if e != nil {
		return pagination.Collection[string]{Items: []string{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

func (c *V1Registries) IterateClusters(ref types.ResourceRef, o pagination.Options) (*pagination.Iterator[ClusterIntegration], error) {
	ref = snapshotRegistryRef(ref)
	id := "registry.v1.registries_clusters.list"
	op, e := c.runtime.BindDocumented(managementOperations[id], c.scope)
	if e != nil {
		return nil, e
	}
	if e = managementValid(ref, c.scope, "v1", "registry"); e != nil {
		return nil, e
	}
	plan, e := c.runtime.PagePlan(id, "v1", c.scope, op.EndpointID, ref)
	if e != nil {
		return nil, e
	}
	plan.FixedSize = true
	return pagination.New(plan, o, func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[ClusterIntegration], error) {
		provider := pagination.ProviderMetadata{}
		var prefix []ClusterIntegration
		r, e := managementCall(ctx, c.runtime, c.scope, id, ref, registryPath(ref.ID)+"/clusters", nil, types.CallOptions{}, func(raw []byte) ([]ClusterIntegration, error) {
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil {
				return nil, types.NewError(types.ProtocolError)
			}
			var total *types.Integer
			if json.Unmarshal(fields["totalCount"], &total) != nil || total == nil {
				return nil, types.NewError(types.ProtocolError)
			}
			count, ok := new(big.Int).SetString(total.String(), 10)
			if !ok || count.Sign() < 0 {
				return nil, types.NewError(types.ProtocolError)
			}
			provider.TotalCount = total
			items, e := registryRows[ClusterIntegration](fields["clusters"], nil)
			prefix = items
			if e != nil {
				return items, e
			}
			if count.Cmp(big.NewInt(int64(len(items)))) != 0 {
				return items, types.NewError(types.Incomplete)
			}
			return items, nil
		})
		if e != nil {
			r.Data = prefix
		}
		return pagination.SourcePage[ClusterIntegration]{Items: r.Data, Metadata: r.Metadata, Provider: provider, Done: e == nil}, e
	})
}
func (c *V1Registries) ClustersPage(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Page[ClusterIntegration], error) {
	it, e := c.IterateClusters(ref, o)
	if e != nil {
		return pagination.Page[ClusterIntegration]{}, e
	}
	return it.Next(ctx)
}
func (c *V1Registries) Clusters(ctx context.Context, ref types.ResourceRef, o pagination.Options) (pagination.Collection[ClusterIntegration], error) {
	it, e := c.IterateClusters(ref, o)
	if e != nil {
		return pagination.Collection[ClusterIntegration]{Items: []ClusterIntegration{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
