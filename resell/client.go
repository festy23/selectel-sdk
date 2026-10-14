// Package resell manages account and Resell resource allocation metadata.
// It is distinct from the Neutron networking API.
package resell

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

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }
func (c *Client) Account(ctx context.Context, options types.CallOptions) (types.Result[Account], error) {
	op := core.Operation{ID: "projects.v2.accounts.list", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[Account], error) {
		var envelope struct {
			Account *Account `json:"account"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Account == nil {
			return core.Decoded[Account]{}, types.NewError(types.ProtocolError)
		}
		if e := envelope.Account.Validate(); e != nil {
			return core.Decoded[Account]{}, e
		}
		return core.Decoded[Account]{Data: *envelope.Account}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/accounts", Options: options, Validate: validate}, decode)
}

func (c *Client) DNSCheck(ctx context.Context, hostname string, options types.CallOptions) (types.Result[DNSInfo], error) {
	op := core.Operation{ID: "projects.v2.dnscheck.get", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}, ErrorCodes: map[int]types.Code{400: types.NotFound}}
	validate := func() error {
		if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
			return types.NewError(types.ScopeMismatch)
		}
		if hostname == "" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	segment := url.PathEscape(hostname)
	if hostname == "." || hostname == ".." {
		segment = strings.ReplaceAll(hostname, ".", "%2E")
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[DNSInfo], error) {
		var envelope struct {
			Result *DNSInfo `json:"result"`
		}
		if json.Unmarshal(raw, &envelope) != nil || envelope.Result == nil {
			return core.Decoded[DNSInfo]{}, types.NewError(types.ProtocolError)
		}
		if e := envelope.Result.Validate(); e != nil {
			return core.Decoded[DNSInfo]{}, e
		}
		return core.Decoded[DNSInfo]{Data: *envelope.Result}, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/v2/dnscheck/" + segment, Options: options, Validate: validate}, decode)
}
