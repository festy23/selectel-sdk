package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

// DirectPortNativeDeleteRequest selects the pinned public-net-api-go variant.
// Force is never sent by current Delete, and false is omitted as in that SDK.
type DirectPortNativeDeleteRequest struct {
	Ref   types.ResourceRef
	Force bool
}

func (c *DirectPorts) DeleteNative(ctx context.Context, req DirectPortNativeDeleteRequest, o types.CallOptions) (types.Result[struct{}], error) {
	in := directRequest(c.scope, req.Ref, o)
	if req.Force {
		in.Query = url.Values{"force": []string{"true"}}
	}
	return core.DoJSON(c.runtime, ctx, directOp("delete", "DELETE"), in, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}

// CreateNative uses the published SDK body without falling back to current API.
func (c *DirectPorts) CreateNative(ctx context.Context, spec DirectPortNativeCreateSpec, o types.CallOptions) (types.Result[DirectPort], error) {
	raw, e := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, directOp("create", "POST"), core.Request{Scope: c.scope, Path: "/v1/public_ports", Body: raw, Options: o, Validate: func() error {
		if e != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.ProjectID != c.scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}, c.decodeDirect(""))
}

// Native list uses the published SDK's optional nonempty query strings.
type DirectPortNativeListOptions struct{ ProjectID, NetworkID, Description string }

func (c *DirectPorts) IterateNative(f DirectPortNativeListOptions, o pagination.Options) (*pagination.Iterator[DirectPort], error) {
	q := url.Values{}
	for key, value := range map[string]string{"project_id": f.ProjectID, "network_id": f.NetworkID, "description": f.Description} {
		if value != "" {
			q.Set(key, value)
		}
	}
	return c.iterateDirect(q, f.ProjectID, o)
}
func (c *DirectPorts) CollectNative(ctx context.Context, f DirectPortNativeListOptions, o pagination.Options) (pagination.Collection[DirectPort], error) {
	it, e := c.IterateNative(f, o)
	if e != nil {
		return pagination.Collection[DirectPort]{Items: []DirectPort{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *DirectPorts) ListPageNative(ctx context.Context, f DirectPortNativeListOptions, o pagination.Options) (pagination.Page[DirectPort], error) {
	it, e := c.IterateNative(f, o)
	if e != nil {
		return pagination.Page[DirectPort]{}, e
	}
	return it.Next(ctx)
}

type DirectPortNativeUpdateRequest struct {
	Ref  types.ResourceRef
	Spec DirectPortNativeUpdateSpec
}

func (c *DirectPorts) UpdateNative(ctx context.Context, req DirectPortNativeUpdateRequest, o types.CallOptions) (types.Result[DirectPort], error) {
	raw, e := json.Marshal(req.Spec)
	in := directRequest(c.scope, req.Ref, o)
	in.Body = raw
	in.Validate = func() error {
		if e != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateDirect(req.Ref)
	}
	return core.DoJSON(c.runtime, ctx, directOp("update", "PATCH"), in, c.decodeDirect(req.Ref.ID, ""))
}
