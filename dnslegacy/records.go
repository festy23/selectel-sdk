package dnslegacy

import (
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Records struct {
	runtime *core.Runtime
	scope   types.Scope
}

func (c *Records) Get(ctx context.Context, parent types.ResourceRef, id string, o types.CallOptions) (types.Result[Record], error) {
	return c.get(ctx, domainTarget{ref: &parent}, id, nil, o)
}
func (c *Records) GetByName(ctx context.Context, name types.ProtectedValue, id string, o types.CallOptions) (types.Result[Record], error) {
	return c.get(ctx, domainTarget{name: &name}, id, nil, o)
}
func (c *Records) GetRef(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Record], error) {
	ref = snapshotRecordRef(ref)
	return c.get(ctx, domainTarget{ref: ref.Parent}, ref.ID, &ref, o)
}
func (c *Records) get(ctx context.Context, target domainTarget, id string, expected *types.ResourceRef, o types.CallOptions) (types.Result[Record], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.get", Service: "dns", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.StaticToken, Success: []int{200}}, c.scope)
	path, _, parentErr := target.request(c.scope, op)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/records/" + id, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if parentErr != nil {
			return parentErr
		}
		if expected != nil {
			_, _, _, e := (recordTarget{ref: expected}).request(c.scope, op)
			if e != nil {
				return e
			}
		}
		if !decimalID.MatchString(id) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, decodeRecord(c.scope, target, id))
}

func decodeRecord(scope types.Scope, target domainTarget, expectedID string) core.Decoder[Record] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[Record], error) {
		var fields struct {
			ID *types.Integer `json:"id"`
		}
		if json.Unmarshal(raw, &fields) != nil || fields.ID == nil || !decimalID.MatchString(fields.ID.String()) {
			return core.Decoded[Record]{}, types.NewError(types.ProtocolError)
		}
		if expectedID != "" && fields.ID.String() != expectedID {
			return core.Decoded[Record]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[Record]{}
		var ref *types.ResourceRef
		if target.ref != nil {
			parent := *target.ref
			known := types.ResourceRef{Scope: scope, Service: "dns", APIGeneration: "v1", Kind: "record", ID: fields.ID.String(), Parent: &parent, EndpointID: ep.ID, EndpointURL: ep.URL}
			ref = &known
			out.Refs = []types.ResourceRef{known}
		}
		var row Record
		if json.Unmarshal(raw, &row) != nil || row.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		row.Ref = ref
		out.Data = row
		return out, nil
	}
}
