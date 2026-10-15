// Package certificates manages project-scoped user TLS certificates.
package certificates

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{r, s} }
func (c *Client) operation(id string) (core.Operation, error) {
	op := core.Operation{ID: id, Service: "certificates", APIGeneration: "user_v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}}
	switch id {
	case "certificates.user_v1.certificates.list", "certificates.user_v1.certificates.get", "certificates.user_v1.private_key.read", "certificates.user_v1.pkcs12.read", "certificates.user_v1.ca_chain.read":
	case "certificates.user_v1.certificates.upload":
		op.Method = "POST"
		op.Effect = "mutation"
		op.Rejected = []int{400, 401, 403}
	case "certificates.user_v1.versions.create":
		op.Method = "POST"
		op.Effect = "mutation"
		op.Success = []int{204}
		op.Rejected = []int{401, 403, 404}
	case "certificates.user_v1.certificates.delete":
		op.Method = "DELETE"
		op.Effect = "mutation"
		op.Success = []int{204}
		op.Rejected = []int{401, 403, 404}
	case "certificates.user_v1.consumers.remove":
		op.Method = "DELETE"
		op.Effect = "mutation"
		op.Success = []int{204}
		op.Rejected = []int{400, 401, 403, 404}
	case "certificates.user_v1.name.update", "certificates.user_v1.consumers.add":
		op.Method = "PUT"
		op.Effect = "mutation"
		op.Success = []int{204}
		op.Rejected = []int{400, 401, 403, 404}
	default:
		return op, types.NewError(types.CapabilityUnavailable)
	}
	return c.runtime.BindDocumented(op, c.scope)
}
func (c *Client) Ref(id string) (types.ResourceRef, error) {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "\r\n\x00") {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("certificates.user_v1.certificates.get")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return types.ResourceRef{Scope: c.scope, Service: "certificates", APIGeneration: "user_v1", Kind: "user_certificate", ID: id, EndpointID: op.EndpointID, EndpointURL: op.FixedEndpoint}, nil
}
func (c *Client) valid(ref types.ResourceRef) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Scope != c.scope || ref.Service != "certificates" || ref.APIGeneration != "user_v1" || ref.Kind != "user_certificate" || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.ID == "." || ref.ID == ".." || strings.ContainsAny(ref.ID, "\r\n\x00") {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func (c *Client) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[Certificate], error) {
	op, bind := c.operation("certificates.user_v1.certificates.get")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cert/" + url.PathEscape(ref.ID), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.valid(ref)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Certificate], error) {
		var out Certificate
		if json.Unmarshal(raw, &out) != nil || out.ID == nil || *out.ID == "" {
			return core.Decoded[Certificate]{}, types.NewError(types.ProtocolError)
		}
		if *out.ID != ref.ID {
			return core.Decoded[Certificate]{}, types.NewError(types.ScopeMismatch)
		}
		out.Ref = *core.Ref(ep, "certificates", "user_certificate", *out.ID)
		return core.Decoded[Certificate]{Data: out}, nil
	})
}
