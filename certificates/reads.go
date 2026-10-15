package certificates

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
)

func (c *Client) read(ctx context.Context, ref types.ResourceRef, operation, suffix string, o types.CallOptions) (types.Result[types.ProtectedValue], error) {
	op, bind := c.operation(operation)
	return core.DoResponse(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cert/" + url.PathEscape(ref.ID) + "/" + suffix, Options: o, Transfer: true, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.valid(ref)
	}}, func(_ context.Context, body io.Reader, _ types.EndpointBinding, _ int, _ http.Header) (core.Decoded[types.ProtectedValue], error) {
		limit := c.runtime.StructuredLimit()
		if limit >= uint64(^uint64(0)>>1) {
			return core.Decoded[types.ProtectedValue]{}, types.NewError(types.ResponseLimit)
		}
		raw, e := io.ReadAll(io.LimitReader(body, int64(limit)+1))
		if e != nil {
			return core.Decoded[types.ProtectedValue]{}, types.NewError(types.TransportError)
		}
		if uint64(len(raw)) > limit {
			return core.Decoded[types.ProtectedValue]{}, types.NewError(types.ResponseLimit)
		}
		return core.Decoded[types.ProtectedValue]{Data: types.Protect(raw)}, nil
	})
}
func (c *Client) ReadPrivateKey(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[types.ProtectedValue], error) {
	return c.read(ctx, ref, "certificates.user_v1.private_key.read", "private_key", o)
}
func (c *Client) ReadPKCS12(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[types.ProtectedValue], error) {
	return c.read(ctx, ref, "certificates.user_v1.pkcs12.read", "p12", o)
}
func (c *Client) ReadPublicChain(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[types.ProtectedValue], error) {
	return c.read(ctx, ref, "certificates.user_v1.ca_chain.read", "ca_chain", o)
}
