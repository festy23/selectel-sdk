package registry

import (
	"context"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type V2Repositories struct{ registries *Registries }
type V2Images struct{ registries *Registries }

func (c V2Client) Repositories() *V2Repositories { return &V2Repositories{c.Registries} }
func (c V2Client) Images() *V2Images             { return &V2Images{c.Registries} }

type ImageListOptions struct {
	Search *string        `json:"search,omitempty"`
	Offset *types.Integer `json:"offset,omitempty"`
}

func (c *V2Repositories) Ref(parent types.ResourceRef, name string) (types.ResourceRef, error) {
	if e := managementValid(parent, c.registries.scope, "v2", "registry"); e != nil {
		return types.ResourceRef{}, e
	}
	return nestedRef(parent, "repository", name)
}
func (c *V2Repositories) Iterate(ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (*pagination.Iterator[V2RepositoryInfo], error) {
	ref = snapshotRegistryRef(ref)
	if f.NativeEmpty204 {
		return nil, types.NewError(types.InvalidArgument)
	}
	q, initial, e := repositoryFilters(f)
	if e != nil {
		return nil, e
	}
	return offsetManagementList(c.registries.runtime, c.registries.scope, "registry.v2.registries_repositories.list", ref, "/v2/registries/"+registrySegment(ref.ID)+"/repositories", q, initial, false, o, func(raw []byte) ([]V2RepositoryInfo, *types.Integer, *types.Integer, error) {
		rows, total, filtered, e := registryEnvelope(raw, "repositories")
		if e != nil {
			return nil, nil, nil, e
		}
		items, e := registryRows(rows, func(v *V2RepositoryInfo) error {
			if v.Name != nil {
				child, e := nestedRef(ref, "repository", *v.Name)
				if e != nil {
					return e
				}
				v.Ref = &child
			}
			return nil
		})
		return items, total, filtered, e
	})
}
func (c *V2Images) Iterate(ref types.ResourceRef, f ImageListOptions, o pagination.Options) (*pagination.Iterator[V2ImageInfo], error) {
	ref = snapshotRegistryRef(ref)
	q, initial, e := repositoryFilters(RepositoryListOptions{Search: f.Search, Offset: f.Offset})
	if e != nil {
		return nil, e
	}
	return offsetManagementList(c.registries.runtime, c.registries.scope, "registry.v2.registries_repositories_images.list", ref, repositoryPath(ref)+"/images", q, initial, false, o, func(raw []byte) ([]V2ImageInfo, *types.Integer, *types.Integer, error) {
		rows, total, filtered, e := registryEnvelope(raw, "images")
		if e != nil {
			return nil, nil, nil, e
		}
		items, e := registryRows(rows, func(v *V2ImageInfo) error {
			if v.Digest != nil {
				child, e := nestedRef(ref, "manifest", *v.Digest)
				if e != nil {
					return e
				}
				v.Ref = &child
			}
			return nil
		})
		return items, total, filtered, e
	})
}

func (c *V2Repositories) ListPage(ctx context.Context, ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (pagination.Page[V2RepositoryInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Page[V2RepositoryInfo]{}, e
	}
	return it.Next(ctx)
}
func (c *V2Repositories) Collect(ctx context.Context, ref types.ResourceRef, f RepositoryListOptions, o pagination.Options) (pagination.Collection[V2RepositoryInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Collection[V2RepositoryInfo]{Items: []V2RepositoryInfo{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

func (c *V2Images) ListPage(ctx context.Context, ref types.ResourceRef, f ImageListOptions, o pagination.Options) (pagination.Page[V2ImageInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Page[V2ImageInfo]{}, e
	}
	return it.Next(ctx)
}
func (c *V2Images) Collect(ctx context.Context, ref types.ResourceRef, f ImageListOptions, o pagination.Options) (pagination.Collection[V2ImageInfo], error) {
	it, e := c.Iterate(ref, f, o)
	if e != nil {
		return pagination.Collection[V2ImageInfo]{Items: []V2ImageInfo{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
