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

func (c *Client) ListADBKeys(ctx context.Context, options pagination.Options) (pagination.Collection[ADBKey], error) {
	op, e := c.binding("mobile_farm.v3.api_keys_adb.list", http.MethodGet, "read", accountOrProject, []int{200}, nil, nil)
	if e != nil {
		return pagination.Collection[ADBKey]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/adb"}, nil, options, "", func(v *ADBKey, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.Fingerprint == nil || *v.Fingerprint == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "adb_key", *v.Fingerprint)
		return nil
	})
	if e != nil {
		return pagination.Collection[ADBKey]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) StoreADBKey(ctx context.Context, input CreateADBKeyRequest, options types.CallOptions) (types.Result[ADBKey], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_keys_adb.create", http.MethodPost, "mutation", accountOrProject, []int{200}, []int{400, 401, 409}, nil)
	body, bodyErr := wireBody(input)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/adb", Body: body, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ADBKey], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ADBKey]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "mobile_farm", "adb_key", *identity.Fingerprint)
		decoded.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		decoded.Data.Ref = ref
		return decoded, nil
	})
	// No direct key Get/status or documented absence proof is available; listing
	// may be explicitly requested but never becomes automatic name-based recovery.
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) DeleteADBKey(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[NoContent], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_keys_adb.delete", http.MethodDelete, "mutation", accountOrProject, []int{204}, []int{400, 401, 404}, []int{204})
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/keys/adb/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, "adb_key", op)
	}}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[NoContent], error) {
		return core.Decoded[NoContent]{}, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) ListUserADBKeys(ctx context.Context, user types.ResourceRef, options pagination.Options) (pagination.Collection[ADBKey], error) {
	op, e := c.binding("mobile_farm.v3.api_users_keys_adb.list", http.MethodGet, "read", accountOnly, []int{200}, nil, nil)
	if e != nil {
		return pagination.Collection[ADBKey]{}, e
	}
	if e := c.validateRef(user, "user", op); e != nil {
		return pagination.Collection[ADBKey]{}, e
	}
	endpoint := types.EndpointBinding{ID: user.EndpointID, URL: user.EndpointURL, Scope: user.Scope, APIGeneration: user.APIGeneration, Audience: "selectel"}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(user.ID) + "/keys/adb", Refs: []types.ResourceRef{user}, PinnedEndpoint: &endpoint}, user, options, "", func(v *ADBKey, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.Fingerprint == nil || *v.Fingerprint == "" {
			return types.NewError(types.ProtocolError)
		}
		v.Ref = core.Ref(ep, "mobile_farm", "adb_key", *v.Fingerprint)
		parent := user
		v.Ref.Parent = &parent
		return nil
	})
	if e != nil {
		return pagination.Collection[ADBKey]{}, e
	}
	return iterator.Collect(ctx)
}

func (c *Client) StoreUserADBKey(ctx context.Context, user types.ResourceRef, input CreateADBKeyRequest, options types.CallOptions) (types.Result[ADBKey], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_keys_adb.create", http.MethodPost, "mutation", accountOnly, []int{200}, []int{400, 401, 403, 404, 409}, nil)
	body, bodyErr := wireBody(input)
	endpoint := types.EndpointBinding{ID: user.EndpointID, URL: user.EndpointURL, Scope: user.Scope, APIGeneration: user.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(user.ID) + "/keys/adb", Body: body, Options: options, Refs: []types.ResourceRef{user}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRef(user, "user", op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ADBKey], error) {
		var identity struct {
			Fingerprint *string `json:"fingerprint"`
		}
		decoded := core.Decoded[ADBKey]{}
		if json.Unmarshal(raw, &identity) != nil || identity.Fingerprint == nil || *identity.Fingerprint == "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "mobile_farm", "adb_key", *identity.Fingerprint)
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

func (c *Client) DeleteUserADBKey(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[NoContent], error) {
	op, bindErr := c.binding("mobile_farm.v3.api_users_keys_adb.delete", http.MethodDelete, "mutation", accountOnly, []int{204}, []int{400, 401, 403, 404}, []int{204})
	userID := ""
	if ref.Parent != nil {
		userID = ref.Parent.ID
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v3/users/" + url.PathEscape(userID) + "/keys/adb/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateUserChild(ref, "adb_key", op)
	}}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[NoContent], error) {
		return core.Decoded[NoContent]{}, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
