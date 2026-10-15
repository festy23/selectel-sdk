package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
)

type GarbageCollectionOptions struct{ DeleteUntagged *bool }

func (c *V1Registries) StartGarbageCollection(ctx context.Context, ref types.ResourceRef, in GarbageCollectionOptions, o types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.runtime.BindDocumented(core.Operation{ID: "registry.v1.registries_garbage_collection.create", Service: "registry", APIGeneration: "v1", Method: "POST", Effect: "mutation", Auth: types.IAMProject, RequireProject: true, Success: []int{201}, NoContentSuccess: []int{201}, Rejected: []int{404, 409}}, c.scope)
	q := url.Values{}
	if in.DeleteUntagged != nil {
		q.Set("delete-untagged", strconv.FormatBool(*in.DeleteUntagged))
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: registryPath(ref.ID) + "/garbage-collection", Query: q, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		return validateRegistryRef(ref)
	}}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *V1Registries) GarbageCollectionSize(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[GarbageCollectionSize], error) {
	op, bind := c.runtime.BindDocumented(core.Operation{ID: "registry.v1.registries_garbage_collection_size.list", Service: "registry", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}}, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: registryPath(ref.ID) + "/garbage-collection/size", Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		return validateRegistryRef(ref)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GarbageCollectionSize], error) {
		var out GarbageCollectionSize
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return core.Decoded[GarbageCollectionSize]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GarbageCollectionSize]{Data: out}, nil
	})
}
