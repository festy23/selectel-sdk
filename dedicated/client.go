// Package dedicated binds explicit Dedicated V2 server lifecycle operations.
package dedicated

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{r, s} }

// Reply distinguishes the typed response schema from the documented
// HTTP 202 generic envelope. Accepted does not imply completed/readiness.
type Reply[T any] struct {
	HTTPStatus   int                 `json:"http_status"`
	ResourceRefs []types.ResourceRef `json:"resource_refs,omitempty"`
	Data         *T                  `json:"data,omitempty"`
	Queued       *ResponseBaseModel  `json:"queued,omitempty"`
	TaskRef      *types.ResourceRef  `json:"task_ref,omitempty"`
}

func segment(s string) string {
	if s == "." || s == ".." {
		return strings.ReplaceAll(s, ".", "%2E")
	}
	return url.PathEscape(s)
}
func (c *Client) refError(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Kind != kind {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "dedicated" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	for p := ref.Parent; p != nil; p = p.Parent {
		if p.Service != ref.Service || p.APIGeneration != ref.APIGeneration || p.EndpointID != ref.EndpointID || p.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func invoke[T any](c *Client, ctx context.Context, id, method, path, kind string, ref *types.ResourceRef, bodySpec interface{ Validate() error }, querySpec interface{ Validate() error }, call types.CallOptions, success []int, async bool, resultKind string) (types.Result[Reply[T]], error) {
	auth := types.IAMAccount
	if c.scope.ProjectID != "" {
		auth = types.IAMProject
	}
	effect := "read"
	if method != "GET" && !isBillingCalculation(id) {
		effect = "mutation"
	}
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: id, Service: "dedicated", APIGeneration: "v2", Method: method, Effect: effect, Auth: auth, Success: success}, c.scope)
	var q url.Values
	var queryErr error
	if querySpec != nil {
		q, queryErr = queryValues(querySpec)
	}
	var body []byte
	var bodyErr error
	if bodySpec != nil {
		body, bodyErr = requestBody(bodySpec)
	}
	var refs []types.ResourceRef
	var pinned *types.EndpointBinding
	if ref != nil {
		refs = []types.ResourceRef{*ref}
		pinned = pin(*ref)
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if queryErr != nil {
			return queryErr
		}
		if kind == "year" {
			if _, e := types.ParseInteger(path[strings.LastIndex(path, "/")+1:]); e != nil {
				return e
			}
		}
		if bodyErr != nil {
			return bodyErr
		}
		if ref != nil {
			if e := c.refError(*ref, kind); e != nil {
				return e
			}
		}
		if bodySpec != nil {
			if target, present := requestProject(bodySpec); present && c.scope.ProjectID != "" && target != c.scope.ProjectID && id != "dedicated.v2.resource_project.replace" {
				return types.NewError(types.ScopeMismatch)
			}
			return inputError(bodySpec)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding, status int) (out core.Decoded[Reply[T]], err error) {
		out.Data.HTTPStatus = status
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return out, types.NewError(types.ProtocolError)
		}
		if !isDirectResponse(id) {
			var taskID *string
			if json.Unmarshal(fields["task_id"], &taskID) != nil || taskID == nil || *taskID == "" {
				return out, types.NewError(types.ProtocolError)
			}
			if kind == "task" && ref != nil && *taskID != ref.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			task := core.Ref(endpoint, "dedicated", "task", *taskID)
			if ref != nil && ref.Kind != "task" {
				copy := cloneRef(*ref)
				task.Parent = &copy
			}
			out.Data.TaskRef = task
			out.Refs = []types.ResourceRef{*task}
		}
		if id == "dedicated.v2.resource_project.replace" && c.scope.ProjectID != "" {
			if target, ok := requestProject(bodySpec); ok && target != c.scope.ProjectID {
				if e := verifyTransfer(fields["result"], ref.ID, target); e != nil {
					return out, e
				}
				resultKind = ""
			}
		}

		if resultKind != "" && status != 202 {
			known, e := scanResultRefs(fields["result"], c.scope, endpoint, resultKind, strictExpected(method, ref))
			out.Refs = append(out.Refs, known...)
			out.Data.ResourceRefs = known
			if e != nil {
				return out, e
			}
		}
		if async && status == 202 {
			var value ResponseBaseModel
			if json.Unmarshal(raw, &value) != nil {
				return out, types.NewError(types.ProtocolError)
			}
			if e := value.Validate(); e != nil {
				return out, e
			}
			out.Data.Queued = &value
			return out, nil
		}
		var value T
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if validator, ok := any(value).(interface{ Validate() error }); ok {
			if e := validator.Validate(); e != nil {
				return out, e
			}
		}
		if isDirectResponse(id) {
			if e := bindDirect(&value, fields, ref); e != nil {
				return out, e
			}
		}
		bindResultRefs(&value, out.Data.ResourceRefs)
		out.Data.Data = &value
		return out, nil
	}
	return core.DoJSONStatus(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Query: q, Body: body, Refs: refs, PinnedEndpoint: pinned, Options: call, Validate: validate}, decode)
}
func (c *Client) SetServerPowerState(ctx context.Context, ref types.ResourceRef, spec PowerSwitchStatus, call types.CallOptions) (types.Result[Reply[ResponseBaseModel]], error) {
	return invoke[ResponseBaseModel](c, ctx, "dedicated.v2.power.replace", "PUT", "/power/"+segment(ref.ID), "resource", &ref, spec, nil, call, []int{200}, false, "")
}
