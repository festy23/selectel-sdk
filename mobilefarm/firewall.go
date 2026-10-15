package mobilefarm

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) ListFirewallRules(ctx context.Context, options pagination.Options) (pagination.Collection[FirewallRule], error) {
	op, e := c.binding("mobile_farm.v3.api_firewall_rules.list", http.MethodGet, "read", projectOnly, []int{200}, nil, nil)
	if e != nil {
		return pagination.Collection[FirewallRule]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/firewall-rules"}, nil, options, "", func(v *FirewallRule, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "firewall_rule", *v.ID)
		return nil
	})
	if e != nil {
		return pagination.Collection[FirewallRule]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) CreateFirewallRule(ctx context.Context, input FirewallRuleRequest, options types.CallOptions) (types.Result[FirewallRule], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_firewall_rules.create", http.MethodPost, "mutation", projectOnly, []int{200, 201}, []int{400, 401, 403, 409}, nil)
	body, bodyErr := wireBody(input)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/firewall-rules", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil || input.Action == nil || input.Address == nil || input.Ports == nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[FirewallRule], error) {
		var identity struct {
			ID *string `json:"id"`
		}
		decoded := core.Decoded[FirewallRule]{}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "mobile_farm", "firewall_rule", *identity.ID)
		decoded.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Ref = ref
		return decoded, nil
	})
	// No direct observation or causal proof permits replay after an unknown outcome.
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) UpdateFirewallRule(ctx context.Context, ref types.ResourceRef, input FirewallRuleRequest, options types.CallOptions) (types.Result[FirewallRule], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_firewall_rules.replace", http.MethodPut, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/firewall-rules/" + url.PathEscape(ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, "firewall_rule", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[FirewallRule], error) {
		var identity struct {
			ID *string `json:"id"`
		}
		decoded := core.Decoded[FirewallRule]{}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.ID != ref.ID {
			return decoded, types.NewError(types.ScopeMismatch)
		}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		copy := ref
		decoded.Data.Ref = &copy
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// DeleteFirewallRule has a source-confirmed 200, but no response schema. It
// always reports Incomplete after acceptance until that contract is available.
func (c *Client) DeleteFirewallRule(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[UnmodeledResponse], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_firewall_rules.delete", http.MethodDelete, "mutation", projectOnly, []int{200}, []int{401, 403, 404}, nil)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/firewall-rules/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "firewall_rule", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[UnmodeledResponse], error) {
		return core.Decoded[UnmodeledResponse]{Data: UnmodeledResponse{HTTPStatus: 200, Body: types.Protect(raw)}}, types.NewError(types.Incomplete)
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// ReorderFirewallRules submits the explicit caller order. The source does not
// guarantee response ordering or that the response is the entire project set.
func (c *Client) ReorderFirewallRules(ctx context.Context, refs []types.ResourceRef, options types.CallOptions) (types.Result[[]FirewallRule], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_firewall_rules_order.update", http.MethodPatch, "mutation", projectOnly, []int{200}, []int{400, 401, 403, 404, 409}, nil)
	ids := make([]string, len(refs))
	for i := range refs {
		ids[i] = refs[i].ID
	}
	body, bodyErr := json.Marshal(struct {
		IDs []string `json:"ids"`
	}{ids})
	var endpoint *types.EndpointBinding
	if len(refs) > 0 {
		r := refs[0]
		endpoint = &types.EndpointBinding{ID: r.EndpointID, URL: r.EndpointURL, Scope: r.Scope, APIGeneration: r.APIGeneration, Audience: "selectel"}
	}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/firewall-rules/order", Body: body, Options: options, Refs: refs, PinnedEndpoint: endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		seen := map[string]bool{}
		for _, ref := range refs {
			if e := c.validateRef(ref, "firewall_rule", op); e != nil {
				return e
			}
			if seen[ref.ID] {
				return types.NewError(types.InvalidArgument)
			}
			seen[ref.ID] = true
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]FirewallRule], error) {
		decoded := core.Decoded[[]FirewallRule]{}
		var rows []json.RawMessage
		if json.Unmarshal(raw, &rows) != nil || rows == nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		seen := map[string]*types.ResourceRef{}
		var firstErr error
		for _, row := range rows {
			var identity struct {
				ID *string `json:"id"`
			}
			if json.Unmarshal(row, &identity) != nil || identity.ID == nil || *identity.ID == "" {
				if firstErr == nil {
					firstErr = types.NewError(types.ProtocolError)
				}
				continue
			}
			if seen[*identity.ID] != nil {
				if firstErr == nil {
					firstErr = types.NewError(types.ProtocolError)
				}
				continue
			}
			ref := core.Ref(ep, "mobile_farm", "firewall_rule", *identity.ID)
			seen[*identity.ID] = ref
			decoded.Refs = append(decoded.Refs, *ref)
		}
		if firstErr != nil {
			return decoded, firstErr
		}
		if json.Unmarshal(raw, &decoded.Data) != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		for i := range decoded.Data {
			rule := &decoded.Data[i]
			if rule.Validate() != nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			rule.Ref = seen[*rule.ID]
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
