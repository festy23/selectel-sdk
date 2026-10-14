// Package publicips keeps Resell allocation, Neutron association and direct
// public network ports as separate API contracts and resource kinds.
package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	DirectPorts *DirectPorts
	FloatingIPs *FloatingIPs
}
type DirectPorts struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{DirectPorts: &DirectPorts{runtime: r, scope: s}, FloatingIPs: &FloatingIPs{runtime: r, scope: s}}
}
func directOp(action, method string) core.Operation {
	op := core.Operation{ID: "public_network.v1.direct_ports." + action, Service: "public_network", APIGeneration: "v1", Method: method, Effect: "read", Auth: types.IAMProject, ServiceType: "public-net-api", ErrorProtocol: "public-network", RequireProject: true, RequireLocation: true, Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403, 404, 409}
	}
	if method == "POST" {
		op.Success = []int{201}
	}
	if method == "DELETE" {
		op.Success = []int{204}
		op.NoContentSuccess = []int{204}
	}
	return op
}
func (c *DirectPorts) Create(ctx context.Context, spec DirectPortCreateSpec, o types.CallOptions) (types.Result[DirectPort], error) {
	raw, e := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, directOp("create", "POST"), core.Request{Scope: c.scope, Path: "/v1/public_ports", Body: raw, Options: o, Validate: func() error {
		if e != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateDirectSpec(spec.Description, spec.SecurityGroupIDs, false)
	}}, c.decodeDirect(""))
}
func (c *DirectPorts) decodeDirect(expected string, owners ...string) core.Decoder[DirectPort] {
	owner := c.scope.ProjectID
	if len(owners) > 0 {
		owner = owners[0]
	}
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[DirectPort], error) {
		out := core.Decoded[DirectPort]{}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		body := envelope["port"]
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if json.Unmarshal(body, &identity) != nil || identity.ID == nil || *identity.ID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		if expected != "" && *identity.ID != expected || owner != "" && identity.ProjectID != nil && *identity.ProjectID != owner {
			return out, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "public_network", "direct_public_port", *identity.ID)
		out.Refs = []types.ResourceRef{*ref}
		var v DirectPort
		if json.Unmarshal(body, &v) != nil || v.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Ref = ref
		out.Data = v
		return out, nil
	}
}
