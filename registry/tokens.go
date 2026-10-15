package registry

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

type V1Tokens struct{ registries *V1Registries }

func (c V1Client) Tokens() *V1Tokens { return &V1Tokens{c.Registries} }

type LegacyTokenCreateOptions struct {
	TTL          *string `json:"ttl,omitempty"`
	DockerConfig *bool   `json:"docker_config,omitempty"`
}

func (c *V1Tokens) Create(ctx context.Context, in LegacyTokenCreateOptions, o types.CallOptions) (types.Result[LegacyToken], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations["registry.v1.token.create"], c.registries.scope)
	q := url.Values{}
	if in.TTL != nil {
		q.Set("ttl", *in.TTL)
	}
	if in.DockerConfig != nil {
		q.Set("docker-config", strconv.FormatBool(*in.DockerConfig))
	}
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: "/v1/token", Query: q, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if in.TTL != nil && *in.TTL != "12h" && *in.TTL != "1y" {
			return types.NewError(types.InvalidArgument)
		}
		if in.DockerConfig != nil && *in.DockerConfig {
			return types.NewError(types.CapabilityUnavailable)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[LegacyToken], error) {
		var out LegacyToken
		if json.Unmarshal(raw, &out) != nil || out.Validate() != nil {
			return core.Decoded[LegacyToken]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[LegacyToken]{Data: out}, nil
	})
}

func legacyTokenCall[T any](ctx context.Context, c *V1Tokens, id string, secret types.ProtectedValue, o types.CallOptions, decode func([]byte) (T, error)) (types.Result[T], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations[id], c.registries.scope)
	value := string(secret.Reveal())
	path := "/v1/token/" + url.PathEscape(value)
	if value == "." || value == ".." {
		path = "/v1/token/" + strings.ReplaceAll(value, ".", "%2E")
	}
	if id == "registry.v1.token_refresh.create" {
		path += "/refresh"
	}
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: path, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if value == "" || !utf8.ValidString(value) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
		out, e := decode(raw)
		return core.Decoded[T]{Data: out}, e
	})
}
func (c *V1Tokens) Get(ctx context.Context, secret types.ProtectedValue, o types.CallOptions) (types.Result[LegacyTokenMetadata], error) {
	return legacyTokenCall(ctx, c, "registry.v1.token.get", secret, o, func(raw []byte) (LegacyTokenMetadata, error) {
		var v LegacyTokenMetadata
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return v, types.NewError(types.ProtocolError)
		}
		return v, nil
	})
}
func (c *V1Tokens) Refresh(ctx context.Context, secret types.ProtectedValue, o types.CallOptions) (types.Result[LegacyToken], error) {
	return legacyTokenCall(ctx, c, "registry.v1.token_refresh.create", secret, o, func(raw []byte) (LegacyToken, error) {
		var v LegacyToken
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil {
			return v, types.NewError(types.ProtocolError)
		}
		return v, nil
	})
}
func (c *V1Tokens) Revoke(ctx context.Context, secret types.ProtectedValue, o types.CallOptions) (types.Result[struct{}], error) {
	return legacyTokenCall(ctx, c, "registry.v1.token.delete", secret, o, func([]byte) (struct{}, error) { return struct{}{}, nil })
}

// CreateDockerConfig explicitly requests the source's Docker-configuration JSON
// variant. The entire configuration is protected; its undocumented internal
// field layout is not interpreted or treated as an exact token identity.
func (c *V1Tokens) CreateDockerConfig(ctx context.Context, ttl *string, o types.CallOptions) (types.Result[types.ProtectedJSON], error) {
	op, bind := c.registries.runtime.BindDocumented(tokenOperations["registry.v1.token.create"], c.registries.scope)
	q := url.Values{"docker-config": {"true"}}
	if ttl != nil {
		q.Set("ttl", *ttl)
	}
	return core.DoJSON(c.registries.runtime, ctx, op, core.Request{Scope: c.registries.scope, Path: "/v1/token", Query: q, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if ttl != nil && *ttl != "12h" && *ttl != "1y" {
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
