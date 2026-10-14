package network

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type Ports struct {
	runtime *core.Runtime
	scope   types.Scope
}
type PortCreateRequest struct {
	Network types.ResourceRef
	Spec    PortCreateSpec
}
type PortUpdateRequest struct {
	Ref      types.ResourceRef
	Spec     PortUpdateSpec
	Revision *types.Integer
}

func (n *Ports) Create(ctx context.Context, req PortCreateRequest, o types.CallOptions) (types.Result[Port], error) {
	owner, ownerErr := requestedOwner(req.Spec.ProjectID, req.Spec.TenantID, n.scope)
	raw, e := json.Marshal(req.Spec)
	body := map[string]json.RawMessage{}
	if e == nil {
		e = json.Unmarshal(raw, &body)
	}
	parentID, _ := json.Marshal(req.Network.ID)
	body["network_id"] = parentID
	raw, e2 := json.Marshal(map[string]any{"port": body})
	in := requestForRef(n.scope, req.Network, "/v2.0/ports", o)
	in.Body = raw
	in.Validate = func() error {
		if ownerErr != nil {
			return ownerErr
		}
		if e != nil || e2 != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRef(req.Network, "network")
	}
	return core.DoJSON(n.runtime, ctx, operation("port", "create", "POST"), in, decodeEnvelope("port", decodeOptions[Port]{Kind: "port", Parent: &req.Network, Owner: owner, SetRef: func(v *Port, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Ports) Get(ctx context.Context, ref types.ResourceRef, opts PortGetOptions, o types.CallOptions) (types.Result[Port], error) {
	q, e := fieldQuery(opts.Fields, validPortField)
	in := requestForRef(n.scope, ref, "/v2.0/ports/"+url.PathEscape(ref.ID), o)
	in.Query = q
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(ref, "port")
	}
	return core.DoJSON(n.runtime, ctx, operation("port", "get", "GET"), in, decodeEnvelope("port", decodeOptions[Port]{Kind: "port", Expected: &ref, Projection: len(opts.Fields) > 0, SetRef: func(v *Port, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Ports) Update(ctx context.Context, req PortUpdateRequest, o types.CallOptions) (types.Result[Port], error) {
	raw, e := json.Marshal(map[string]any{"port": req.Spec})
	op := operation("port", "update", "PUT")
	headerErr := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/ports/"+url.PathEscape(req.Ref.ID), o)
	in.Body = raw
	in.Validate = func() error {
		if headerErr != nil {
			return headerErr
		}
		if e != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRef(req.Ref, "port")
	}
	return core.DoJSON(n.runtime, ctx, op, in, decodeEnvelope("port", decodeOptions[Port]{Kind: "port", Expected: &req.Ref, SetRef: func(v *Port, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Ports) Delete(ctx context.Context, req DeleteRequest, o types.CallOptions) (types.Result[struct{}], error) {
	op := operation("port", "delete", "DELETE")
	e := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/ports/"+url.PathEscape(req.Ref.ID), o)
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(req.Ref, "port")
	}
	return core.DoJSON(n.runtime, ctx, op, in, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
