// Package quotas manages project quotas through the regional Quota Management API.
package quotas

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct{ V1 *V1Client }
type V1Client struct {
	runtime *core.Runtime
	scope   types.Scope
}
type Filters struct{ Resources []string }

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{V1: &V1Client{runtime: r, scope: s}}
}
func (c *V1Client) operation(id, method string) (core.Operation, error) {
	op := core.Operation{ID: id, Service: "quotas", APIGeneration: "v1", Method: method, Effect: "read", Auth: types.IAMProject, Alternatives: []types.AuthMode{types.IAMAccount}, ProjectAccountAuth: true, RequireProject: true, RequireLocation: true, Success: []int{200}}
	if id != "quotas.v1.limits.get" {
		op.Success = []int{200, 207}
		op.PartialSuccess = []int{207}
	}
	if method == "PATCH" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403}
	}
	return c.runtime.BindDocumented(op, c.scope)
}
func (c *V1Client) request(bindErr error, suffix string, f Filters, o types.CallOptions) core.Request {
	query := url.Values{}
	for _, r := range f.Resources {
		if strings.TrimSpace(r) == "" {
			bindErr = types.NewError(types.InvalidArgument)
		}
		query.Add("resource", r)
	}
	return core.Request{Scope: c.scope, Path: "/v1/projects/" + url.PathEscape(c.scope.ProjectID) + suffix, Query: query, Options: o, Validate: func() error { return bindErr }}
}
func (c *V1Client) Get(ctx context.Context, f Filters, o types.CallOptions) (types.Result[GetResponse], error) {
	op, err := c.operation("quotas.v1.quotas.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, c.request(err, "/quotas", f, o), func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetResponse], error) {
		var v GetResponse
		if json.Unmarshal(raw, &v) != nil || (v.Quotas == nil && v.Error == nil && len(v.Errors) == 0) {
			return core.Decoded[GetResponse]{}, types.NewError(types.ProtocolError)
		}
		if err := v.Validate(); err != nil {
			return core.Decoded[GetResponse]{}, err
		}
		if v.Quotas == nil {
			v.Quotas = map[string][]Quota{}
		}
		d := core.Decoded[GetResponse]{Data: v}
		if v.Error != nil || len(v.Errors) > 0 {
			d.PartialRead = true
			return d, types.NewError(types.Incomplete)
		}
		return d, nil
	})
}
func (c *V1Client) Update(ctx context.Context, v UpdateRequest, o types.CallOptions) (types.Result[UpdateResponse], error) {
	op, err := c.operation("quotas.v1.quotas.update", "PATCH")
	req := c.request(err, "/quotas", Filters{}, o)
	if err == nil {
		req.Refs = []types.ResourceRef{{ID: c.scope.ProjectID, Kind: "project_quotas", Service: "quotas", APIGeneration: "v1", Scope: c.scope, EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}}
	}
	body, bodyErr := json.Marshal(v)
	req.Body = body
	req.Validate = func() error {
		if err != nil {
			return err
		}
		if bodyErr != nil || len(v.Quotas) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		for resource, values := range v.Quotas {
			if strings.TrimSpace(resource) == "" || len(values) == 0 {
				return types.NewError(types.InvalidArgument)
			}
			for _, q := range values {
				if q.Value == nil || q.Value.String() == "" {
					return types.NewError(types.InvalidArgument)
				}
			}
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, op, req, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UpdateResponse], error) {
		var v UpdateResponse
		if json.Unmarshal(raw, &v) != nil || (v.Quotas == nil && v.Error == nil && len(v.Errors) == 0) {
			return core.Decoded[UpdateResponse]{}, types.NewError(types.ProtocolError)
		}
		if err := v.Validate(); err != nil {
			return core.Decoded[UpdateResponse]{}, err
		}
		if v.Quotas == nil {
			v.Quotas = map[string][]Quota{}
		}
		d := core.Decoded[UpdateResponse]{Data: v}
		if v.Error != nil || len(v.Errors) > 0 {
			d.PartialRead = true
			return d, types.NewError(types.Incomplete)
		}
		return d, nil
	})
}

func (c *V1Client) Limits(ctx context.Context, f Filters, o types.CallOptions) (types.Result[LimitsResponse], error) {
	op, err := c.operation("quotas.v1.limits.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, c.request(err, "/limits", f, o), func(raw []byte, _ types.EndpointBinding) (core.Decoded[LimitsResponse], error) {
		var v LimitsResponse
		if json.Unmarshal(raw, &v) != nil || v.Quotas == nil {
			return core.Decoded[LimitsResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[LimitsResponse]{Data: v}, v.Validate()
	})
}
