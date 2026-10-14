// Package backups implements the explicitly selected Scheduled Backup API.
package backups

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime     *core.Runtime
	scope       types.Scope
	Plans       *PlansClient
	Checkpoints *CheckpointsClient
	Restores    *RestoresClient
	SDKV2       *SDKV2Client
}
type PlansClient struct{ client *Client }

func Bind(r *core.Runtime, scope types.Scope) *Client {
	c := &Client{runtime: r, scope: scope}
	c.Plans = &PlansClient{client: c}
	c.Checkpoints = &CheckpointsClient{client: c}
	c.Restores = &RestoresClient{client: c}
	c.SDKV2 = &SDKV2Client{client: c}
	return c
}
func segment(s string) string {
	if s == "." || s == ".." {
		return strings.ReplaceAll(s, ".", "%2E")
	}
	return url.PathEscape(s)
}
func (c *Client) path(resource string) string {
	return "/" + segment(c.scope.ProjectID) + "/" + resource
}
func (c *Client) operation(resource, action, method string) core.Operation {
	op := core.Operation{ID: "backups.v2." + resource + "." + action, Service: "backups", APIGeneration: "v2", Method: method, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 403, 404, 422}
	}
	return op
}
func (c *PlansClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Plan], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("plans", "get", "GET"), cl.scope)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		if ref.Kind != "plan" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Plan], error) {
		var v Plan
		if json.Unmarshal(raw, &v) != nil {
			return core.Decoded[Plan]{}, types.NewError(types.ProtocolError)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[Plan]{}, e
		}
		if *v.ID != ref.ID {
			return core.Decoded[Plan]{}, types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(endpoint, "backups", "plan", ref.ID)
		return core.Decoded[Plan]{Data: v}, nil
	}
	pin := &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("plans/") + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin, Options: options, Validate: validate}, decode)
}
