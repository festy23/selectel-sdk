package dnslegacy

import (
	"context"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Records) Create(ctx context.Context, parent types.ResourceRef, spec RecordCreateSpec, o types.CallOptions) (types.Result[Record], error) {
	return c.create(ctx, domainTarget{ref: &parent}, spec, o)
}
func (c *Records) CreateByName(ctx context.Context, name types.ProtectedValue, spec RecordCreateSpec, o types.CallOptions) (types.Result[Record], error) {
	return c.create(ctx, domainTarget{name: &name}, spec, o)
}
func (c *Records) create(ctx context.Context, target domainTarget, spec RecordCreateSpec, o types.CallOptions) (types.Result[Record], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.create", Service: "dns", APIGeneration: "v1", Method: "POST", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, _, parentErr := target.request(c.scope, op)
	body, bodyErr := wireBody(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if parentErr != nil {
			return parentErr
		}
		if spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path + "/records", Body: body, Options: o, Validate: validate}, decodeRecord(c.scope, target, ""))
}

func (c *Records) Update(ctx context.Context, ref types.ResourceRef, spec RecordUpdateSpec, o types.CallOptions) (types.Result[Record], error) {
	ref = snapshotRecordRef(ref)
	return c.update(ctx, recordTarget{ref: &ref}, spec, o)
}
func (c *Records) UpdateByName(ctx context.Context, parentName types.ProtectedValue, id string, spec RecordUpdateSpec, o types.CallOptions) (types.Result[Record], error) {
	return c.update(ctx, recordTarget{parentName: &parentName, id: id}, spec, o)
}
func (c *Records) update(ctx context.Context, target recordTarget, spec RecordUpdateSpec, o types.CallOptions) (types.Result[Record], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.replace_1158c7f1", Service: "dns", APIGeneration: "v1", Method: "PUT", Effect: "mutation", Auth: types.StaticToken, Success: []int{200}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, parent, refs, targetErr := target.request(c.scope, op)
	body, bodyErr := wireBody(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if targetErr != nil {
			return targetErr
		}
		if spec.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Refs: refs, Body: body, Options: o, Validate: validate}, decodeRecord(c.scope, parent, target.recordID()))
}

func (c *Records) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	ref = snapshotRecordRef(ref)
	return c.delete(ctx, recordTarget{ref: &ref}, o)
}
func (c *Records) DeleteByName(ctx context.Context, parentName types.ProtectedValue, id string, o types.CallOptions) (types.Result[struct{}], error) {
	return c.delete(ctx, recordTarget{parentName: &parentName, id: id}, o)
}
func (c *Records) delete(ctx context.Context, target recordTarget, o types.CallOptions) (types.Result[struct{}], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.records.delete", Service: "dns", APIGeneration: "v1", Method: "DELETE", Effect: "mutation", Auth: types.StaticToken, Success: []int{204}, NoContentSuccess: []int{204}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	path, _, refs, targetErr := target.request(c.scope, op)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Refs: refs, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return targetErr
	}}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
