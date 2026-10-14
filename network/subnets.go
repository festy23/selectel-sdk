package network

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type Subnets struct {
	runtime *core.Runtime
	scope   types.Scope
}
type SubnetCreateRequest struct {
	Network types.ResourceRef
	Spec    SubnetCreateSpec
}
type SubnetUpdateRequest struct {
	Ref      types.ResourceRef
	Spec     SubnetUpdateSpec
	Revision *types.Integer
}

func (n *Subnets) Create(ctx context.Context, req SubnetCreateRequest, o types.CallOptions) (types.Result[Subnet], error) {
	owner, ownerErr := requestedOwner(req.Spec.ProjectID, req.Spec.TenantID, n.scope)
	raw, e := json.Marshal(req.Spec)
	body := map[string]json.RawMessage{}
	if e == nil {
		e = json.Unmarshal(raw, &body)
	}
	parentID, _ := json.Marshal(req.Network.ID)
	body["network_id"] = parentID
	raw, e2 := json.Marshal(map[string]any{"subnet": body})
	in := requestForRef(n.scope, req.Network, "/v2.0/subnets", o)
	in.Body = raw
	in.Validate = func() error {
		if ownerErr != nil {
			return ownerErr
		}
		if e != nil || e2 != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if req.Spec.IPVersion.String() != "4" && req.Spec.IPVersion.String() != "6" {
			return types.NewError(types.InvalidArgument)
		}
		return validateRef(req.Network, "network")
	}
	return core.DoJSON(n.runtime, ctx, operation("subnet", "create", "POST"), in, decodeEnvelope("subnet", decodeOptions[Subnet]{Kind: "subnet", Parent: &req.Network, Owner: owner, SetRef: func(v *Subnet, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Subnets) Get(ctx context.Context, ref types.ResourceRef, opts SubnetGetOptions, o types.CallOptions) (types.Result[Subnet], error) {
	q, e := fieldQuery(opts.Fields, validSubnetField)
	in := requestForRef(n.scope, ref, "/v2.0/subnets/"+url.PathEscape(ref.ID), o)
	in.Query = q
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(ref, "subnet")
	}
	return core.DoJSON(n.runtime, ctx, operation("subnet", "get", "GET"), in, decodeEnvelope("subnet", decodeOptions[Subnet]{Kind: "subnet", Expected: &ref, Projection: len(opts.Fields) > 0, SetRef: func(v *Subnet, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Subnets) Update(ctx context.Context, req SubnetUpdateRequest, o types.CallOptions) (types.Result[Subnet], error) {
	raw, e := json.Marshal(map[string]any{"subnet": req.Spec})
	op := operation("subnet", "update", "PUT")
	headerErr := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/subnets/"+url.PathEscape(req.Ref.ID), o)
	in.Body = raw
	in.Validate = func() error {
		if headerErr != nil {
			return headerErr
		}
		if e != nil || req.Spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return validateRef(req.Ref, "subnet")
	}
	return core.DoJSON(n.runtime, ctx, op, in, decodeEnvelope("subnet", decodeOptions[Subnet]{Kind: "subnet", Expected: &req.Ref, SetRef: func(v *Subnet, r *types.ResourceRef) { v.Ref = r }}))
}
func (n *Subnets) Delete(ctx context.Context, req DeleteRequest, o types.CallOptions) (types.Result[struct{}], error) {
	op := operation("subnet", "delete", "DELETE")
	e := revisionHeader(&op, req.Revision)
	in := requestForRef(n.scope, req.Ref, "/v2.0/subnets/"+url.PathEscape(req.Ref.ID), o)
	in.Validate = func() error {
		if e != nil {
			return e
		}
		return validateRef(req.Ref, "subnet")
	}
	return core.DoJSON(n.runtime, ctx, op, in, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
