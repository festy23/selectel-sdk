package backups

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"reflect"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// SDKV2Client explicitly selects the wire variant published by cloudbackup-go.
// It never falls back from the current API or aliases retention field names.
type SDKV2Client struct{ client *Client }
type SDKPlansQuery struct {
	Name, VolumeName, Marker *string
	Limit                    *types.Integer
}
type SDKCheckpointsQuery struct {
	PlanName, VolumeName, Marker *string
	Limit                        *types.Integer
}

func (c *SDKV2Client) operation(resource, action, method string) (core.Operation, error) {
	op := c.client.operation("sdk_"+resource, action, method)
	op.Rejected = nil
	return c.client.runtime.BindDocumented(op, c.client.scope)
}
func sdkQuery(limit *types.Integer, fields map[string]*string) (url.Values, error) {
	q := url.Values{}
	if limit != nil {
		value, e := types.ParseInteger(limit.String())
		if e != nil {
			return nil, e
		}
		if value.String() != "0" {
			q.Set("limit", value.String())
		}
	}
	for key, v := range fields {
		if v != nil && *v != "" {
			q.Set(key, *v)
		}
	}
	return q, nil
}
func (c *SDKV2Client) PlansPage(ctx context.Context, filters SDKPlansQuery, options types.CallOptions) (types.Result[SDKPlansPage], error) {
	op, bindingErr := c.operation("plans", "list", "GET")
	q, queryErr := sdkQuery(filters.Limit, map[string]*string{"name": filters.Name, "volume_name": filters.VolumeName, "marker": filters.Marker})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return queryErr
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[SDKPlansPage], error) {
		var value SDKPlansPage
		if json.Unmarshal(raw, &value) != nil || value.Plans == nil {
			return core.Decoded[SDKPlansPage]{}, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[SDKPlansPage]{}, e
		}
		for i := range value.Plans {
			if value.Plans[i].ID != nil {
				value.Plans[i].Ref = core.Ref(ep, "backups", "sdk_plan", *value.Plans[i].ID)
			}
		}
		return core.Decoded[SDKPlansPage]{Data: value}, nil
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/plans/", Query: q, Options: options, Validate: validate}, decode)
}
func (c *SDKV2Client) CheckpointsPage(ctx context.Context, filters SDKCheckpointsQuery, options types.CallOptions) (types.Result[SDKCheckpointsPage], error) {
	op, bindingErr := c.operation("checkpoints", "list", "GET")
	q, queryErr := sdkQuery(filters.Limit, map[string]*string{"plan_name": filters.PlanName, "volume_name": filters.VolumeName, "marker": filters.Marker})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return queryErr
	}
	decode := func(raw []byte, ep types.EndpointBinding) (core.Decoded[SDKCheckpointsPage], error) {
		var value SDKCheckpointsPage
		if json.Unmarshal(raw, &value) != nil || value.Checkpoints == nil {
			return core.Decoded[SDKCheckpointsPage]{}, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[SDKCheckpointsPage]{}, e
		}
		for i := range value.Checkpoints {
			row := &value.Checkpoints[i]
			if row.ID != nil {
				row.Ref = core.Ref(ep, "backups", "sdk_checkpoint", *row.ID)
			}
		}
		return core.Decoded[SDKCheckpointsPage]{Data: value}, nil
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/checkpoints/", Query: q, Options: options, Validate: validate}, decode)
}
func sdkBody(spec any, defaults []string) ([]byte, error) {
	v := reflect.ValueOf(spec)
	if v.Kind() == reflect.Pointer && v.IsNil() {
		return []byte("null"), nil
	}
	wire, e := wireValue(v)
	if e != nil {
		return nil, e
	}
	fields := wire.(map[string]any)
	for _, key := range defaults {
		if _, exists := fields[key]; !exists {
			switch key {
			case "full_backups_amount":
				fields[key] = 0
			case "resources":
				fields[key] = nil
			default:
				fields[key] = ""
			}
		}
	}
	for _, key := range []string{"id", "created_at", "status"} {
		if value, ok := fields[key].(string); ok && value == "" {
			delete(fields, key)
		}
	}
	raw, e := json.Marshal(fields)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	return raw, nil
}
func (c *SDKV2Client) planDecoder(expected string) core.Decoder[SDKPlan] {
	return func(raw []byte, ep types.EndpointBinding) (out core.Decoded[SDKPlan], err error) {
		var id struct {
			ID *string `json:"id"`
		}
		if json.Unmarshal(raw, &id) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		actual := expected
		if id.ID != nil && *id.ID != "" {
			if expected != "" && expected != *id.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			actual = *id.ID
		}
		if actual == "" {
			return out, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "backups", "sdk_plan", actual)
		out.Refs = []types.ResourceRef{*ref}
		var value SDKPlan
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
}
func (c *SDKV2Client) PlanCreate(ctx context.Context, spec *SDKPlan, options types.CallOptions) (types.Result[SDKPlan], error) {
	op, bindingErr := c.operation("plans", "create", "POST")
	op.Success = []int{201}
	body, bodyErr := sdkBody(spec, []string{"backup_mode", "full_backups_amount", "name", "resources", "schedule_pattern", "schedule_type"})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return bodyErr
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/plans/", Body: body, Options: options, Validate: validate}, c.planDecoder(""))
}
func (c *SDKV2Client) PlanGet(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[SDKPlan], error) {
	op, bindingErr := c.operation("plans", "get", "GET")
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return c.client.validateRef(ref, "sdk_plan")
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/plans/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: validate}, c.planDecoder(ref.ID))
}
func (c *SDKV2Client) PlanUpdate(ctx context.Context, ref types.ResourceRef, spec *SDKPlanUpdateSpec, options types.CallOptions) (types.Result[SDKPlan], error) {
	op, bindingErr := c.operation("plans", "update", "PATCH")
	body, bodyErr := sdkBody(spec, []string{"full_backups_amount", "name", "resources", "schedule_pattern", "schedule_type"})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		if e := c.client.validateRef(ref, "sdk_plan"); e != nil {
			return e
		}
		return bodyErr
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/plans/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Body: body, Options: options, Validate: validate}, c.planDecoder(ref.ID))
}
func (c *SDKV2Client) PlanDelete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	op, bindingErr := c.operation("plans", "delete", "DELETE")
	op.Success = []int{204}
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return c.client.validateRef(ref, "sdk_plan")
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[struct{}]{}, nil
	}
	return core.DoJSON(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: "/plans/" + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: validate}, decode)
}
