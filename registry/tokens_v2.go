package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"strings"
)

type V2Tokens struct{ registries *Registries }

func (c V2Client) Tokens() *V2Tokens { return &V2Tokens{c.Registries} }
func tokenRefValid(ref types.ResourceRef, scope types.Scope) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != scope || ref.Service != "registry" || ref.APIGeneration != "v2" {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "token" || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func tokenPath(id string) string {
	part := url.PathEscape(id)
	if id == "." || id == ".." {
		part = strings.ReplaceAll(id, ".", "%2E")
	}
	return "/v2/tokens/" + part
}
func decodeToken(expected string) core.Decoder[TokenInfo] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[TokenInfo], error) {
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil {
			return core.Decoded[TokenInfo]{}, types.NewError(types.ProtocolError)
		}
		var id string
		if json.Unmarshal(fields["id"], &id) != nil || id == "" {
			return core.Decoded[TokenInfo]{}, types.NewError(types.ProtocolError)
		}
		if expected != "" && id != expected {
			return core.Decoded[TokenInfo]{}, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "registry", "token", id)
		if ref == nil {
			return core.Decoded[TokenInfo]{}, types.NewError(types.ProtocolError)
		}
		out := core.Decoded[TokenInfo]{Refs: []types.ResourceRef{*ref}}
		var v TokenInfo
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Ref = ref
		out.Data = v
		return out, nil
	}
}
func (c *V2Tokens) Create(ctx context.Context, in TokenCreateRequest, o types.CallOptions) (types.Result[TokenInfo], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations["registry.v2.tokens.create"], c.registries.scope)
	body, e := json.Marshal(in)
	q := url.Values{}
	if in.DockerConfig != nil {
		q.Set("docker-config", strconv.FormatBool(*in.DockerConfig))
	}
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: "/v2/tokens", Body: body, Query: q, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if e != nil || in.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if in.DockerConfig != nil && *in.DockerConfig {
			return types.NewError(types.CapabilityUnavailable)
		}
		return nil
	}}, decodeToken(""))
}
func (c *V2Tokens) Get(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[TokenInfo], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations["registry.v2.tokens.get"], c.registries.scope)
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: tokenPath(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		return tokenRefValid(ref, c.registries.scope)
	}}, decodeToken(ref.ID))
}

func v2TokenMutation[T any](ctx context.Context, c *V2Tokens, id string, ref types.ResourceRef, in any, o types.CallOptions, decode core.Decoder[T]) (types.Result[T], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations[id], c.registries.scope)
	var body []byte
	var bodyErr error
	if in != nil {
		body, bodyErr = json.Marshal(in)
	}
	path := tokenPath(ref.ID)
	switch id {
	case "registry.v2.tokens_refresh.create":
		path += "/refresh"
	case "registry.v2.tokens_regenerate.create":
		path += "/regenerate"
	case "registry.v2.tokens_revoke.create":
		path += "/revoke"
	}
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: path, Body: body, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: registryPin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		if bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if v, ok := in.(interface{ Validate() error }); ok && v.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return tokenRefValid(ref, c.registries.scope)
	}}, decode)
}
func (c *V2Tokens) Refresh(ctx context.Context, ref types.ResourceRef, in TokenRefreshRequest, o types.CallOptions) (types.Result[TokenInfo], error) {
	return v2TokenMutation(ctx, c, "registry.v2.tokens_refresh.create", ref, in, o, decodeToken(ref.ID))
}
func (c *V2Tokens) Regenerate(ctx context.Context, ref types.ResourceRef, in TokenRegenerateRequest, o types.CallOptions) (types.Result[TokenInfo], error) {
	return v2TokenMutation(ctx, c, "registry.v2.tokens_regenerate.create", ref, in, o, decodeToken(ref.ID))
}
func (c *V2Tokens) Update(ctx context.Context, ref types.ResourceRef, in TokenUpdateRequest, o types.CallOptions) (types.Result[TokenInfo], error) {
	return v2TokenMutation(ctx, c, "registry.v2.tokens.update", ref, in, o, decodeToken(ref.ID))
}
func (c *V2Tokens) Revoke(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return v2TokenMutation(ctx, c, "registry.v2.tokens_revoke.create", ref, nil, o, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
func (c *V2Tokens) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	return v2TokenMutation(ctx, c, "registry.v2.tokens.delete", ref, nil, o, func([]byte, types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}

// CreateDockerConfig selects the documented JSON configuration variant without
// interpreting its source-undocumented internal schema or inventing a token ID.
func (c *V2Tokens) CreateDockerConfig(ctx context.Context, in TokenCreateRequest, o types.CallOptions) (types.Result[types.ProtectedJSON], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations["registry.v2.tokens.create"], c.registries.scope)
	body, bodyErr := json.Marshal(in)
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: "/v2/tokens", Query: url.Values{"docker-config": {"true"}}, Body: body, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if bodyErr != nil || in.Validate() != nil || (in.DockerConfig != nil && !*in.DockerConfig) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[types.ProtectedJSON], error) {
		var v types.ProtectedJSON
		if json.Unmarshal(raw, &v) != nil {
			return core.Decoded[types.ProtectedJSON]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[types.ProtectedJSON]{Data: v}, nil
	})
}
