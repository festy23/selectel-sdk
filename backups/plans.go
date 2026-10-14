package backups

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *PlansClient) Create(ctx context.Context, spec PlanCreateSpec, options types.CallOptions) (types.Result[Plan], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("plans", "create", "POST"), cl.scope)
	op.Success = []int{201}
	op.Rejected = []int{400, 403, 422}
	body, bodyErr := requestBody(spec)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		if e := inputError(spec); e != nil {
			return e
		}
		if bodyErr != nil {
			return bodyErr
		}
		if *spec.BackupMode != "full" && *spec.BackupMode != "frequency" {
			return types.NewError(types.InvalidArgument)
		}
		if *spec.ScheduleType != "calendar" && *spec.ScheduleType != "crontab" {
			return types.NewError(types.InvalidArgument)
		}
		if _, e := types.ParseInteger(spec.MaxBackups.String()); e != nil {
			return e
		}
		if !validResources(spec.Resources) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[Plan], err error) {
		var fields struct {
			ID *string `json:"id"`
		}
		if json.Unmarshal(raw, &fields) != nil || fields.ID == nil || *fields.ID == "" {
			return out, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(endpoint, "backups", "plan", *fields.ID)
		out.Refs = []types.ResourceRef{*ref}
		var value Plan
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return out, e
		}
		value.Ref = ref
		out.Data = value
		return out, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("plans/"), Body: body, Options: options, Validate: validate}, decode)
}
func (c *PlansClient) Update(ctx context.Context, ref types.ResourceRef, spec PlanUpdateSpec, options types.CallOptions) (types.Result[PlanUpdateResponse], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("plans", "update", "PATCH"), cl.scope)
	op.Success = []int{200, 204}
	op.NoContentSuccess = []int{204}
	op.Rejected = []int{400, 404, 422}
	body, bodyErr := requestBody(spec)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		if ref.Kind != "plan" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := inputError(spec); e != nil {
			return e
		}
		if !optionalEnum(spec.Status, "started", "suspended") || !optionalEnum(spec.ScheduleType, "calendar", "crontab") {
			return types.NewError(types.InvalidArgument)
		}
		if spec.MaxBackups != nil {
			if value, present := spec.MaxBackups.Value(); present {
				if _, e := types.ParseInteger(value.String()); e != nil {
					return e
				}
			}
		}
		if spec.Resources != nil {
			if value, present := spec.Resources.Value(); present && !validResources(value) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return bodyErr
	}
	decode := func(raw []byte, _ types.EndpointBinding) (out core.Decoded[PlanUpdateResponse], err error) {
		if len(raw) == 0 {
			out.Data.Ref = &ref
			return out, nil
		}
		var value PlanUpdateResponse
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return out, e
		}
		value.Ref = &ref
		out.Data = value
		return out, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("plans/") + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Body: body, Options: options, Validate: validate}, decode)
}

func optionalEnum(value *Optional[string], allowed ...string) bool {
	if value == nil {
		return true
	}
	actual, present := value.Value()
	if !present {
		return true
	}
	for _, item := range allowed {
		if actual == item {
			return true
		}
	}
	return false
}

func validResources(resources []Resource) bool {
	for _, item := range resources {
		if !optionalEnum(item.Type, "OS::Cinder::Volume") {
			return false
		}
	}
	return true
}

func (c *PlansClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("plans", "delete", "DELETE"), cl.scope)
	op.Success = []int{204}
	op.Rejected = []int{403, 404, 422}
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return cl.validateRef(ref, "plan")
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("plans/") + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: validate}, decode)
}
