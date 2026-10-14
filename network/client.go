// Package network binds source-confirmed Neutron v2 network resources.
package network

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	Networks *Networks
	Subnets  *Subnets
	Ports    *Ports
}
type Networks struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{Networks: &Networks{runtime: r, scope: s}, Subnets: &Subnets{runtime: r, scope: s}, Ports: &Ports{runtime: r, scope: s}}
}
func operation(resource, action, method string) core.Operation {
	op := core.Operation{ID: "network.v2." + resource + "s." + action, Service: "network", APIGeneration: "v2", Method: method, Effect: "read", Auth: types.IAMProject, ServiceType: "network", RequireProject: true, RequireLocation: true, Success: []int{200}}
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

// An explicit target owner is distinct from the caller's project auth view.
func requestedOwner(project, tenant *string, scope types.Scope) (string, error) {
	if project != nil && *project == "" || tenant != nil && *tenant == "" {
		return "", types.NewError(types.InvalidArgument)
	}
	if project != nil && tenant != nil && *project != *tenant {
		return "", types.NewError(types.ScopeMismatch)
	}
	if project != nil {
		return *project, nil
	}
	if tenant != nil {
		return *tenant, nil
	}
	return scope.ProjectID, nil
}
func responseOwner(project, tenant *string, expected string) error {
	if project != nil && tenant != nil && *project != *tenant {
		return types.NewError(types.ScopeMismatch)
	}
	owner := ""
	if project != nil {
		owner = *project
	} else if tenant != nil {
		owner = *tenant
	}
	if expected != "" && owner != "" && owner != expected {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (n *Networks) Create(ctx context.Context, spec NetworkCreateSpec, o types.CallOptions) (types.Result[Network], error) {
	owner, validation := requestedOwner(spec.ProjectID, spec.TenantID, n.scope)
	raw, encodeErr := json.Marshal(map[string]any{"network": spec})
	return core.DoJSON(n.runtime, ctx, operation("network", "create", "POST"), core.Request{Scope: n.scope, Path: "/v2.0/networks", Body: raw, Options: o, Validate: func() error {
		if validation != nil {
			return validation
		}
		if encodeErr != nil || spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decodeEnvelope("network", decodeOptions[Network]{Kind: "network", Owner: owner, SetRef: func(v *Network, r *types.ResourceRef) { v.Ref = r }}))
}

func (n *Networks) Get(ctx context.Context, ref types.ResourceRef, options NetworkGetOptions, o types.CallOptions) (types.Result[Network], error) {
	q, e := fieldQuery(options.Fields, validNetworkField)
	in := requestForRef(n.scope, ref, "/v2.0/networks/"+url.PathEscape(ref.ID), o)
	in.Query = q
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(ref, "network")
	}
	return core.DoJSON(n.runtime, ctx, operation("network", "get", "GET"), in, decodeEnvelope("network", decodeOptions[Network]{Kind: "network", Expected: &ref, Projection: len(options.Fields) > 0, SetRef: func(v *Network, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Networks) Update(ctx context.Context, req NetworkUpdateRequest, o types.CallOptions) (types.Result[Network], error) {
	raw, e := json.Marshal(map[string]any{"network": req.Spec})
	op := operation("network", "update", "PUT")
	headerErr := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/networks/"+url.PathEscape(req.Ref.ID), o)
	in.Body = raw
	in.Validate = func() error {
		if headerErr != nil {
			return headerErr
		}
		if e != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRef(req.Ref, "network")
	}
	return core.DoJSON(n.runtime, ctx, op, in, decodeEnvelope("network", decodeOptions[Network]{Kind: "network", Expected: &req.Ref, SetRef: func(v *Network, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Networks) Delete(ctx context.Context, req DeleteRequest, o types.CallOptions) (types.Result[struct{}], error) {
	op := operation("network", "delete", "DELETE")
	e := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/networks/"+url.PathEscape(req.Ref.ID), o)
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(req.Ref, "network")
	}
	return core.DoJSON(n.runtime, ctx, op, in, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
