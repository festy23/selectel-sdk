package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"unicode/utf8"
)

type DirectPortUpdateRequest struct {
	Ref  types.ResourceRef
	Spec DirectPortUpdateSpec
}
type DirectPortListOptions struct{ NetworkID, Description, Subnet *string }

func validateDirect(ref types.ResourceRef) error {
	if ref.Service != "public_network" || ref.APIGeneration != "v1" || ref.Kind != "direct_public_port" || ref.ID == "" || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func pinned(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func directRequest(scope types.Scope, ref types.ResourceRef, o types.CallOptions) core.Request {
	return core.Request{Scope: scope, Path: "/v1/public_ports/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pinned(ref), Options: o, Validate: func() error { return validateDirect(ref) }}
}
func validateDirectSpec(description *types.Nullable[string], groups []string, update bool) error {
	if description != nil {
		if text, ok := description.Value(); ok && utf8.RuneCountInString(text) > 5000 {
			return types.NewError(types.InvalidArgument)
		}
	}
	if len(groups) > 20 || update && groups != nil && len(groups) == 0 {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (c *DirectPorts) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[DirectPort], error) {
	return core.DoJSON(c.runtime, ctx, directOp("get", "GET"), directRequest(c.scope, ref, o), c.decodeDirect(ref.ID, ""))
}
func (c *DirectPorts) Update(ctx context.Context, req DirectPortUpdateRequest, o types.CallOptions) (types.Result[DirectPort], error) {
	raw, e := json.Marshal(req.Spec)
	in := directRequest(c.scope, req.Ref, o)
	in.Body = raw
	in.Validate = func() error {
		if e != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := validateDirectSpec(req.Spec.Description, req.Spec.SecurityGroupIDs, true); e != nil {
			return e
		}
		return validateDirect(req.Ref)
	}
	return core.DoJSON(c.runtime, ctx, directOp("update", "PATCH"), in, c.decodeDirect(req.Ref.ID, ""))
}
func (c *DirectPorts) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return core.DoJSON(c.runtime, ctx, directOp("delete", "DELETE"), directRequest(c.scope, ref, o), func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *DirectPorts) Quotas(ctx context.Context, o types.CallOptions) (types.Result[DirectQuotas], error) {
	op := directOp("get", "GET")
	op.ID = "public_network.v1.quotas.get"
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/projects/" + url.PathEscape(c.scope.ProjectID) + "/quotas", Options: o}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[DirectQuotas], error) {
		var value DirectQuotas
		if string(raw) == "null" || json.Unmarshal(raw, &value) != nil || value.Validate() != nil {
			return core.Decoded[DirectQuotas]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[DirectQuotas]{Data: value}, nil
	})
}
func (c *DirectPorts) Iterate(f DirectPortListOptions, o pagination.Options) (*pagination.Iterator[DirectPort], error) {
	q := url.Values{}
	for key, value := range map[string]*string{"network_id": f.NetworkID, "description": f.Description, "subnet": f.Subnet} {
		if value != nil {
			q.Set(key, *value)
		}
	}
	return c.iterateDirect(q, "", o)
}
func (c *DirectPorts) iterateDirect(q url.Values, owner string, o pagination.Options) (*pagination.Iterator[DirectPort], error) {
	op := directOp("list", "GET")
	plan, e := c.runtime.PagePlan(op.ID, "v1", c.scope, "", q)
	if e != nil {
		return nil, e
	}
	plan.FixedSize = true
	plan.DynamicEndpoint = plan.EndpointID == ""
	expected := plan.EndpointID
	if o.Cursor != nil {
		expected = o.Cursor.EndpointID
	}
	fetch := func(ctx context.Context, _ string, _ uint) (pagination.SourcePage[DirectPort], error) {
		result, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v1/public_ports", Query: q, ExpectedEndpointID: expected}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]DirectPort], error) {
			out := core.Decoded[[]DirectPort]{Data: []DirectPort{}}
			var body struct {
				Ports []json.RawMessage `json:"ports"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Ports == nil {
				return out, types.NewError(types.ProtocolError)
			}
			for _, item := range body.Ports {
				envelope, _ := json.Marshal(map[string]json.RawMessage{"port": item})
				decoded, e := c.decodeDirect("", owner)(envelope, ep)
				if e != nil {
					return out, e
				}
				out.Data = append(out.Data, decoded.Data)
			}
			return out, nil
		})
		if e == nil {
			expected = result.Metadata.EndpointID
		}
		return pagination.SourcePage[DirectPort]{Items: result.Data, Done: true, Metadata: result.Metadata}, e
	}
	return pagination.New(plan, o, fetch)
}
func (c *DirectPorts) Collect(ctx context.Context, f DirectPortListOptions, o pagination.Options) (pagination.Collection[DirectPort], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[DirectPort]{Items: []DirectPort{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *DirectPorts) ListPage(ctx context.Context, f DirectPortListOptions, o pagination.Options) (pagination.Page[DirectPort], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[DirectPort]{}, e
	}
	return it.Next(ctx)
}
