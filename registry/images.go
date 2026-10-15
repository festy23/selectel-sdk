package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
)

type V1Images struct{ registries *V1Registries }

func (c V1Client) Images() *V1Images { return &V1Images{c.Registries} }
func (c *V1Images) Reference(parent types.ResourceRef, reference string) (types.ResourceRef, error) {
	if e := managementValid(parent, c.registries.scope, "v1", "repository"); e != nil {
		return types.ResourceRef{}, e
	}
	return nestedRef(parent, "image_reference", reference)
}
func (c *V1Images) Manifest(parent types.ResourceRef, digest string) (types.ResourceRef, error) {
	if e := managementValid(parent, c.registries.scope, "v1", "repository"); e != nil {
		return types.ResourceRef{}, e
	}
	return nestedRef(parent, "manifest", digest)
}
func (c *V1Images) Layers(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ImageInfo], error) {
	path := ""
	if ref.Parent != nil {
		path = repositoryPath(*ref.Parent) + "/" + registrySegment(ref.ID)
	}
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.get_94cf85a5", ref, path, nil, o, func(raw []byte) (ImageInfo, error) {
		var out ImageInfo
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if ref.Kind == "manifest" && out.Digest != nil && *out.Digest != ref.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		out.Ref = &ref
		return out, nil
	})
}
func (c *V1Images) DeleteManifest(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	path := ""
	if ref.Parent != nil {
		path = repositoryPath(*ref.Parent) + "/" + registrySegment(ref.ID)
	}
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.delete_9116f718", ref, path, nil, o, func([]byte) (struct{}, error) { return struct{}{}, nil })
}

// NativeLayers selects the array response documented by the pinned public Go SDK.
// It does not retry or fall back from the current API's Image-object response.
func (c *V1Images) NativeLayers(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[[]ImageLayer], error) {
	path := ""
	if ref.Parent != nil {
		path = repositoryPath(*ref.Parent) + "/" + registrySegment(ref.ID)
	}
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.get_94cf85a5", ref, path, nil, o, func(raw []byte) ([]ImageLayer, error) {
		var out []ImageLayer
		if json.Unmarshal(raw, &out) != nil || out == nil {
			return nil, types.NewError(types.ProtocolError)
		}
		for _, v := range out {
			if v.Validate() != nil {
				return nil, types.NewError(types.ProtocolError)
			}
		}
		return out, nil
	})
}
