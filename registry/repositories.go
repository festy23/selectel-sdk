package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
	"unicode/utf8"
)

type V1Repositories struct{ registries *V1Registries }

func (c V1Client) Repositories() *V1Repositories { return &V1Repositories{c.Registries} }
func managementValid(ref types.ResourceRef, scope types.Scope, generation, kind string) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != scope || ref.Service != "registry" || ref.APIGeneration != generation || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "registry" {
		return validateRegistryRef(ref)
	}
	parentKind := "registry"
	if kind == "manifest" || kind == "image_reference" {
		parentKind = "repository"
	}
	if ref.Parent == nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.EndpointID != ref.Parent.EndpointID || ref.EndpointURL != ref.Parent.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return managementValid(*ref.Parent, scope, generation, parentKind)
}
func registrySegment(s string) string {
	if s == "." || s == ".." {
		return strings.ReplaceAll(s, ".", "%2E")
	}
	return url.PathEscape(s)
}
func repositoryPath(ref types.ResourceRef) string {
	if ref.Parent == nil {
		return ""
	}
	return "/" + ref.APIGeneration + "/registries/" + registrySegment(ref.Parent.ID) + "/repositories/" + registrySegment(ref.ID)
}
func snapshotRegistryRef(ref types.ResourceRef) types.ResourceRef {
	raw, _ := json.Marshal(ref)
	var saved types.ResourceRef
	_ = json.Unmarshal(raw, &saved)
	return saved
}
func nestedRef(parent types.ResourceRef, kind, id string) (types.ResourceRef, error) {
	if id == "" || !utf8.ValidString(id) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	saved := snapshotRegistryRef(parent)
	ref := types.ResourceRef{Scope: parent.Scope, Service: "registry", APIGeneration: parent.APIGeneration, Kind: kind, ID: id, EndpointID: parent.EndpointID, EndpointURL: parent.EndpointURL, Parent: &saved}
	return ref, nil
}
func (c *V1Repositories) Ref(parent types.ResourceRef, name string) (types.ResourceRef, error) {
	if e := managementValid(parent, c.registries.scope, "v1", "registry"); e != nil {
		return types.ResourceRef{}, e
	}
	return nestedRef(parent, "repository", name)
}
func managementCall[T any](ctx context.Context, runtime *core.Runtime, scope types.Scope, id string, ref types.ResourceRef, path string, body []byte, o types.CallOptions, decode func([]byte) (T, error)) (types.Result[T], error) {
	definition := managementOperations[id]
	op, bind := runtime.BindDocumented(definition, scope)
	return core.DoJSON(runtime, ctx, op, core.Request{Scope: scope, Path: path, Body: body, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		kind := managementRefKinds[id]
		if id == "registry.v1.registries_repositories.get_94cf85a5" && ref.Kind == "manifest" {
			kind = "manifest"
		}
		return managementValid(ref, scope, op.APIGeneration, kind)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
		out, e := decode(raw)
		return core.Decoded[T]{Data: out}, e
	})
}
func (c *V1Repositories) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[RepositoryInfo], error) {
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.get", ref, repositoryPath(ref), nil, o, func(raw []byte) (RepositoryInfo, error) {
		var out RepositoryInfo
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if out.Name != nil && *out.Name != ref.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		out.Ref = &ref
		return out, nil
	})
}
func (c *V1Repositories) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories.delete", ref, repositoryPath(ref), nil, o, func([]byte) (struct{}, error) { return struct{}{}, nil })
}

func (c *V1Repositories) Cleanup(ctx context.Context, ref types.ResourceRef, in CleanupOptions, o types.CallOptions) (types.Result[CleanupResult], error) {
	body, _ := json.Marshal(in)
	return managementCall(ctx, c.registries.runtime, c.registries.scope, "registry.v1.registries_repositories_cleanup.create", ref, repositoryPath(ref)+"/cleanup", body, o, func(raw []byte) (CleanupResult, error) {
		var out CleanupResult
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if out.Failed != nil && len(*out.Failed) > 0 {
			e := types.NewError(types.ProviderError)
			e.Reason = "partial_success"
			return out, e
		}
		return out, nil
	})
}
