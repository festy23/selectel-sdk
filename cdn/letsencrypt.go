package cdn

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) OrderWildcardLetsEncrypt(ctx context.Context, spec LetsEncryptWildcardOrderRequest, o types.CallOptions) (types.Result[LetsEncryptWildcardOrderResponse], error) {
	op, bindErr := c.operation("cdn.v3.letsencrypt.order_wildcard", "POST")
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/letsencrypt", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return encodeErr
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[LetsEncryptWildcardOrderResponse], error) {
		var out LetsEncryptWildcardOrderResponse
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[LetsEncryptWildcardOrderResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[LetsEncryptWildcardOrderResponse]{Data: out}, logicalStatus(out.Status, "LetsEncryptWildcardOrderResponse")
	})
}
func (c *Client) OrderLetsEncrypt(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[LetsEncryptOrderResponse], error) {
	op, bindErr := c.operation("cdn.v3.letsencrypt.order", "POST")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/letsencrypt/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "resource")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[LetsEncryptOrderResponse], error) {
		var out LetsEncryptOrderResponse
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[LetsEncryptOrderResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[LetsEncryptOrderResponse]{Data: out}, logicalStatus(out.Status, "LetsEncryptOrderResponse")
	})
}
func (c *Client) GetLetsEncryptStatus(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[LetsEncryptStatusResponse], error) {
	op, bindErr := c.operation("cdn.v3.letsencrypt.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/letsencrypt/" + url.PathEscape(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "resource")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[LetsEncryptStatusResponse], error) {
		var out LetsEncryptStatusResponse
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[LetsEncryptStatusResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[LetsEncryptStatusResponse]{Data: out}, logicalStatus(out.Status, "LetsEncryptStatusResponse")
	})
}
