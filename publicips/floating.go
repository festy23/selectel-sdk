package publicips

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

type FloatingIPs struct {
	runtime *core.Runtime
	scope   types.Scope
}
type FloatingIPCreateRequest struct {
	Network types.ResourceRef
	Port    *types.ResourceRef
	Spec    FloatingIPCreateSpec
}
type FloatingIPUpdateRequest struct {
	Ref      types.ResourceRef
	Port     *types.Nullable[types.ResourceRef]
	Spec     FloatingIPUpdateSpec
	Revision *types.Integer
}

func floatingOp(action, method string) core.Operation {
	op := core.Operation{ID: "network.v2.floatingips." + action, Service: "network", APIGeneration: "v2", Auth: types.IAMProject, Method: method, Effect: "read", ServiceType: "network", RequireProject: true, RequireLocation: true, Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403, 404, 409, 412}
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
func validateNetworkRef(ref types.ResourceRef, kind string) error {
	if ref.ID == "" || ref.Service != "network" || ref.APIGeneration != "v2" || ref.Kind != kind {
		return types.NewError(types.ScopeMismatch)
	}
	if kind == "network" {
		if ref.Parent != nil {
			return types.NewError(types.ScopeMismatch)
		}
	} else {
		if ref.Parent == nil || validateNetworkRef(*ref.Parent, "network") != nil || ref.Parent.Scope != ref.Scope || ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func sameNetworkView(a, b types.ResourceRef) bool {
	return a.Scope == b.Scope && a.EndpointID == b.EndpointID && a.EndpointURL == b.EndpointURL
}
func (c *FloatingIPs) Create(ctx context.Context, req FloatingIPCreateRequest, o types.CallOptions) (types.Result[FloatingIP], error) {
	raw, e := json.Marshal(req.Spec)
	body := map[string]json.RawMessage{}
	if e == nil {
		e = json.Unmarshal(raw, &body)
	}
	body["floating_network_id"], _ = json.Marshal(req.Network.ID)
	refs := []types.ResourceRef{req.Network}
	if req.Port != nil {
		body["port_id"], _ = json.Marshal(req.Port.ID)
		refs = append(refs, *req.Port)
	}
	raw, e2 := json.Marshal(map[string]any{"floatingip": body})
	validate := func() error {
		if e != nil || e2 != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := validateNetworkRef(req.Network, "network"); e != nil {
			return e
		}
		if req.Port != nil && (validateNetworkRef(*req.Port, "port") != nil || !sameNetworkView(req.Network, *req.Port)) {
			return types.NewError(types.ScopeMismatch)
		}
		if req.Spec.ProjectID != nil && req.Spec.TenantID != nil && *req.Spec.ProjectID != *req.Spec.TenantID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	owner := c.scope.ProjectID
	if req.Spec.ProjectID != nil {
		owner = *req.Spec.ProjectID
	} else if req.Spec.TenantID != nil {
		owner = *req.Spec.TenantID
	}
	in := core.Request{Scope: c.scope, Path: "/v2.0/floatingips", Body: raw, Options: o, Refs: refs, PinnedEndpoint: pinned(req.Network), Validate: validate}
	return core.DoJSON(c.runtime, ctx, floatingOp("create", "POST"), in, c.decodeFloating(nil, &req.Network, owner, false))
}
func (c *FloatingIPs) Update(ctx context.Context, req FloatingIPUpdateRequest, o types.CallOptions) (types.Result[FloatingIP], error) {
	raw, e := json.Marshal(req.Spec)
	body := map[string]json.RawMessage{}
	if e == nil {
		e = json.Unmarshal(raw, &body)
	}
	refs := []types.ResourceRef{req.Ref}
	var port *types.ResourceRef
	if req.Port != nil {
		if value, ok := req.Port.Value(); ok {
			port = &value
			refs = append(refs, value)
			body["port_id"], _ = json.Marshal(value.ID)
		} else {
			body["port_id"] = json.RawMessage("null")
		}
	}
	raw, e2 := json.Marshal(map[string]any{"floatingip": body})
	op := floatingOp("update", "PUT")
	revisionErr := revisionCondition(&op, req.Revision)
	in := core.Request{Scope: c.scope, Path: "/v2.0/floatingips/" + url.PathEscape(req.Ref.ID), Body: raw, Options: o, Refs: refs, PinnedEndpoint: pinned(req.Ref)}
	in.Validate = func() error {
		if e != nil || e2 != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := validateNetworkRef(req.Ref, "floating_ip"); e != nil {
			return e
		}
		if port != nil && (validateNetworkRef(*port, "port") != nil || !sameNetworkView(req.Ref, *port)) {
			return types.NewError(types.ScopeMismatch)
		}
		return revisionErr
	}
	return core.DoJSON(c.runtime, ctx, op, in, c.decodeFloating(&req.Ref, nil, "", false))
}
func revisionCondition(op *core.Operation, revision *types.Integer) error {
	if revision == nil {
		return nil
	}
	if revision.String() == "" || revision.String()[0] == '-' {
		return types.NewError(types.InvalidArgument)
	}
	op.Headers = http.Header{"If-Match": []string{"revision_number=" + revision.String()}}
	return nil
}
func (c *FloatingIPs) decodeFloating(expected, parent *types.ResourceRef, owner string, projection bool) core.Decoder[FloatingIP] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[FloatingIP], error) {
		out := core.Decoded[FloatingIP]{}
		var env map[string]json.RawMessage
		if json.Unmarshal(raw, &env) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		raw = env["floatingip"]
		var identity struct {
			ID        *string `json:"id"`
			NetworkID *string `json:"floating_network_id"`
			ProjectID *string `json:"project_id"`
			TenantID  *string `json:"tenant_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID != nil && *identity.ID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		id := ""
		if identity.ID != nil {
			id = *identity.ID
		}
		if id == "" && expected != nil && projection {
			id = expected.ID
		}
		if id == "" && !projection {
			return out, types.NewError(types.ProtocolError)
		}
		if identity.ProjectID != nil && identity.TenantID != nil && *identity.ProjectID != *identity.TenantID {
			return out, types.NewError(types.ScopeMismatch)
		}
		actualOwner := identity.ProjectID
		if actualOwner == nil {
			actualOwner = identity.TenantID
		}
		if owner != "" && actualOwner != nil && *actualOwner != owner {
			return out, types.NewError(types.ScopeMismatch)
		}
		if expected != nil {
			if id != expected.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			parent = expected.Parent
		}
		if identity.NetworkID != nil {
			if *identity.NetworkID == "" {
				return out, types.NewError(types.ProtocolError)
			}
			if parent != nil && *identity.NetworkID != parent.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			parent = core.Ref(ep, "network", "network", *identity.NetworkID)
		}
		if parent == nil && !projection {
			return out, types.NewError(types.ProtocolError)
		}
		var ref *types.ResourceRef
		if id != "" && parent != nil {
			ref = core.Ref(ep, "network", "floating_ip", id)
			copy := *parent
			ref.Parent = &copy
			out.Refs = []types.ResourceRef{*ref}
		}
		var v FloatingIP
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Ref = ref
		out.Data = v
		return out, nil
	}
}
