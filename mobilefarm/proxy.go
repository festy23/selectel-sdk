package mobilefarm

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"
)

func (c *Client) ListProxyTokens(ctx context.Context, options pagination.Options) (pagination.Collection[ProxyTokenInfo], error) {
	op, e := c.binding("mobile_farm.v3.api_keys_proxy.list", http.MethodGet, "read", accountOrProject, []int{200}, nil, nil)
	if e != nil {
		return pagination.Collection[ProxyTokenInfo]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/proxy"}, nil, options, "", func(v *ProxyTokenInfo, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.Fingerprint == nil || *v.Fingerprint == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "proxy_token", *v.Fingerprint)
		return nil
	})
	if e != nil {
		return pagination.Collection[ProxyTokenInfo]{}, e
	}
	return iterator.Collect(ctx)
}

func validProxyName(name *string) bool {
	if name == nil {
		return false
	}
	length := utf8.RuneCountInString(strings.TrimSpace(*name))
	return length >= 1 && length <= 255
}
func (c *Client) CreateProxyToken(ctx context.Context, input CreateProxyTokenRequest, options types.CallOptions) (types.Result[ProxyToken], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_keys_proxy.create", http.MethodPost, "mutation", accountOrProject, []int{200}, []int{400, 401, 409}, nil)
	body, bodyErr := wireBody(input)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/proxy", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil || !validProxyName(input.Name) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ProxyToken], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ProxyToken]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "mobile_farm", "proxy_token", *identity.Fingerprint)
		decoded.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Ref = ref
		return decoded, nil
	})
	// Token secret is single-delivery; never recreate or name-search on unknown.
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) DeleteProxyToken(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[ProxyTokenInfo], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_keys_proxy.delete", http.MethodDelete, "mutation", accountOrProject, []int{200}, []int{401, 404}, nil)
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/proxy/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "proxy_token", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[ProxyTokenInfo], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ProxyTokenInfo]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.Fingerprint != ref.ID {
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

func (c *Client) ListUserProxyTokens(ctx context.Context, user types.ResourceRef, options pagination.Options) (pagination.Collection[ProxyTokenInfo], error) {
	op, e := c.binding("mobile_farm.v3.api_users_keys_proxy.list", http.MethodGet, "read", accountOnly, []int{200}, nil, nil)
	if e != nil {
		return pagination.Collection[ProxyTokenInfo]{}, e
	}
	if e := c.validateRef(user, "user", op); e != nil {
		return pagination.Collection[ProxyTokenInfo]{}, e
	}
	endpoint := types.EndpointBinding{ID: user.EndpointID, URL: user.EndpointURL, Scope: user.Scope, APIGeneration: user.APIGeneration, Audience: "selectel"}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(user.ID) + "/keys/proxy", Refs: []types.ResourceRef{user}, PinnedEndpoint: &endpoint}, user, options, "", func(v *ProxyTokenInfo, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.Fingerprint == nil || *v.Fingerprint == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "proxy_token", *v.Fingerprint)
		parent := user
		v.Ref.Parent = &parent
		return nil
	})
	if e != nil {
		return pagination.Collection[ProxyTokenInfo]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) CreateUserProxyToken(ctx context.Context, user types.ResourceRef, input CreateProxyTokenRequest, options types.CallOptions) (types.Result[ProxyToken], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_keys_proxy.create", http.MethodPost, "mutation", accountOnly, []int{200}, []int{400, 401, 403, 404, 409, 429}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: user.EndpointID, URL: user.EndpointURL, Scope: user.Scope, APIGeneration: user.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(user.ID) + "/keys/proxy", Body: body, Options: options, Refs: []types.ResourceRef{user}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRef(user, "user", op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil || !validProxyName(input.Name) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ProxyToken], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ProxyToken]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "mobile_farm", "proxy_token", *identity.Fingerprint)
		parent := user
		ref.Parent = &parent
		decoded.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Ref = ref
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) DeleteUserProxyToken(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[ProxyTokenInfo], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_keys_proxy.delete", http.MethodDelete, "mutation", accountOnly, []int{200}, []int{401, 403, 404}, nil)
	userID := ""
	if ref.Parent != nil {
		userID = ref.Parent.ID
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(userID) + "/keys/proxy/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateUserChild(ref, "proxy_token", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[ProxyTokenInfo], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ProxyTokenInfo]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		if *identity.Fingerprint != ref.ID {
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
