// Package ptr binds account reverse DNS, independently from public/private zones.
package ptr

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net"
	"unicode/utf8"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}
type Empty struct{}

func Bind(r *core.Runtime, s types.Scope) *Client { return &Client{r, s} }
func op(action, method string) core.Operation {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	status := 200
	if method == "DELETE" {
		status = 204
	}
	return core.Operation{ID: "ip_addresses.v1.ptrs." + action, Service: "ip_addresses", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.StaticToken, EndpointID: "selectel-ipam", FixedEndpoint: "https://api.selectel.ru/ipam/v1", Success: []int{status}, Rejected: rejectedStatuses[action], ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 422: types.InvalidArgument}}
}
func (c *Client) validScope() error {
	if c.scope.Validate() != nil || c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func decoder(expected string) core.Decoder[Record] {
	return func(raw []byte, ep types.EndpointBinding) (d core.Decoded[Record], e error) {
		var identity struct {
			ID *types.Integer `json:"id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		id := identity.ID.String()
		if expected != "" && id != expected {
			return d, types.NewError(types.ScopeMismatch)
		}
		r := core.Ref(ep, "ip_addresses", "ptr", id)
		d.Refs = []types.ResourceRef{*r}
		if e = json.Unmarshal(raw, &d.Data); e != nil {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Ref = r
		return d, nil
	}
}
func (c *Client) Create(ctx context.Context, in CreateRequest, o types.CallOptions) (types.Result[Record], error) {
	body := map[string]any{"ip": in.IP}
	if in.Content != nil {
		body["content"] = string(in.Content.Reveal())
	}
	if in.TTL != nil {
		body["ttl"] = in.TTL
	}
	b, _ := json.Marshal(body)
	return core.DoJSON(c.runtime, ctx, op("create", "POST"), core.Request{Scope: c.scope, Path: "/", Body: b, Options: o, Validate: func() error {
		if e := c.validScope(); e != nil {
			return e
		}
		if net.ParseIP(in.IP) == nil {
			return types.NewError(types.InvalidArgument)
		}
		if in.Content != nil {
			v := string(in.Content.Reveal())
			if !utf8.ValidString(v) || utf8.RuneCountInString(v) < minContent || utf8.RuneCountInString(v) > maxContent {
				return types.NewError(types.InvalidArgument)
			}
		}
		if in.TTL != nil {
			v, ok := new(big.Int).SetString(in.TTL.String(), 10)
			if !ok || v.Cmp(big.NewInt(minTTL)) < 0 || v.Cmp(big.NewInt(maxTTL)) > 0 {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, decoder(""))
}
func (c *Client) valid(r types.ResourceRef) error {
	if e := c.validScope(); e != nil {
		return e
	}
	if r.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if r.Scope != c.scope || r.Service != "ip_addresses" || r.APIGeneration != "v1" || r.Kind != "ptr" || r.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if _, e := types.ParseInteger(r.ID); e != nil {
		return e
	}
	return nil
}
func pin(r types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
}
func (c *Client) request(r types.ResourceRef, o types.CallOptions) core.Request {
	return core.Request{Scope: c.scope, Path: "/" + r.ID, Options: o, Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Validate: func() error { return c.valid(r) }}
}
func (c *Client) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Record], error) {
	return core.DoJSON(c.runtime, ctx, op("get", "GET"), c.request(r, o), decoder(r.ID))
}
func (c *Client) Update(ctx context.Context, r types.ResourceRef, in UpdateRequest, o types.CallOptions) (types.Result[Record], error) {
	req := c.request(r, o)
	body := map[string]any{}
	if in.IP != nil {
		body["ip"] = *in.IP
	}
	if in.Content != nil {
		body["content"] = string(in.Content.Reveal())
	}
	req.Body, _ = json.Marshal(body)
	previous := req.Validate
	req.Validate = func() error {
		if e := previous(); e != nil {
			return e
		}
		if in.Content != nil && !utf8.Valid(in.Content.Reveal()) {
			return types.NewError(types.InvalidArgument)
		}
		if in.IP != nil && net.ParseIP(*in.IP) == nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	return core.DoJSON(c.runtime, ctx, op("update", "PUT"), req, decoder(r.ID))
}
func (c *Client) Delete(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Empty], error) {
	return core.DoJSON(c.runtime, ctx, op("delete", "DELETE"), c.request(r, o), func([]byte, types.EndpointBinding) (core.Decoded[Empty], error) { return core.Decoded[Empty]{}, nil })
}
