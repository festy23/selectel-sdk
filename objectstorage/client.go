// Package objectstorage manages the project-scoped Object Storage control API.
package objectstorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"unicode/utf8"
)

type Client struct {
	runtime    *core.Runtime
	scope      types.Scope
	Account    *Account
	Containers *Containers
	LogExports *LogExports
}
type Account struct{ client *Client }

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Account = &Account{c}
	c.Containers = &Containers{c}
	c.LogExports = &LogExports{c}
	return c
}
func (c *Account) SetQuota(ctx context.Context, in SetQuotaOptions, options types.CallOptions) (types.Result[Quota], error) {
	s := c.client
	op, bind := s.runtime.BindDocumented(core.Operation{ID: "object_storage.v2.account_quotas.set", Service: "object_storage", APIGeneration: "v2", Method: "PUT", Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}, Rejected: []int{401}}, s.scope)
	body, encode := json.Marshal(in)
	return core.DoJSON(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: "/v2/account/quotas", Body: body, Options: options, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return types.NewError(types.InvalidArgument)
		}
		if in.QuotaBytes != nil {
			if v, ok := in.QuotaBytes.Value(); ok {
				n, valid := new(big.Int).SetString(v.String(), 10)
				if !valid || n.Sign() < 0 || n.BitLen() > 64 {
					return types.NewError(types.InvalidArgument)
				}
			}
		}
		return nil
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Quota], error) {
		var out Quota
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[Quota]{}, types.NewError(types.ProtocolError)
		}
		if e := out.validate(); e != nil {
			return core.Decoded[Quota]{}, e
		}
		return core.Decoded[Quota]{Data: out}, nil
	})
}

func (c *Account) GetQuota(ctx context.Context, o types.CallOptions) (types.Result[Quota], error) {
	return readAccount[Quota](ctx, c.client, "account_quotas.get", "/v2/account/quotas", o)
}
func (c *Account) Info(ctx context.Context, o types.CallOptions) (types.Result[AccountInfo], error) {
	return readAccount[AccountInfo](ctx, c.client, "account.info", "/v2/account/info", o)
}
func readAccount[T any](ctx context.Context, c *Client, id, path string, o types.CallOptions, validation ...func() error) (types.Result[T], error) {
	op, bind := c.runtime.BindDocumented(core.Operation{ID: "object_storage.v2." + id, Service: "object_storage", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Options: o, Validate: func() error { return bind }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
		var out T
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &out) != nil {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		if validator, ok := any(out).(interface{ validate() error }); ok {
			if e := validator.validate(); e != nil {
				return core.Decoded[T]{}, e
			}
		}
		return core.Decoded[T]{Data: out}, nil
	})
}

type Containers struct{ client *Client }

func (c *Containers) Ref(name string) (types.ResourceRef, error) {
	if name == "" || !utf8.ValidString(name) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.client.runtime.BindDocumented(operationBindings["container_domains.list"], c.client.scope)
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.client.scope, APIGeneration: "v2", Audience: "selectel"}, "object_storage", "container_name", name), nil
}
func (c *Client) validateRef(ref types.ResourceRef, kind string) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != c.scope || ref.Service != "object_storage" || ref.APIGeneration != "v2" || ref.Kind != kind || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *Containers) Domains(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Domains], error) {
	return call[Domains](ctx, c.client, "container_domains.list", "/v2/containers/"+url.PathEscape(r.ID)+"/domains", &r, nil, o)
}
func (c *Containers) AddDomain(ctx context.Context, r types.ResourceRef, in AddDomainOptions, o types.CallOptions) (types.Result[Empty], error) {
	body, _ := json.Marshal(in)
	return call[Empty](ctx, c.client, "container_domains.add", "/v2/containers/"+url.PathEscape(r.ID)+"/domains", &r, body, o)
}

type Empty struct{}

func call[T any](ctx context.Context, c *Client, id, path string, ref *types.ResourceRef, body []byte, o types.CallOptions, validation ...func() error) (types.Result[T], error) {
	op, bind := c.runtime.BindDocumented(operationBindings[id], c.scope)
	request := core.Request{Scope: c.scope, Path: path, Body: body, Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		for _, validate := range validation {
			if e := validate(); e != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		if ref != nil {
			return c.validateRef(*ref, "container_name")
		}
		return nil
	}}
	if ref != nil {
		request.Refs = []types.ResourceRef{*ref}
		request.PinnedEndpoint = &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: "v2", Audience: "selectel"}
	}
	return core.DoJSON(c.runtime, ctx, op, request, func(raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
		var out T
		if len(raw) == 0 && len(op.NoContentSuccess) == 0 {
			return core.Decoded[T]{}, types.NewError(types.ProtocolError)
		}
		if len(raw) > 0 {
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(raw, &out) != nil {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
		}
		if validator, ok := any(out).(interface{ validate() error }); ok {
			if e := validator.validate(); e != nil {
				return core.Decoded[T]{}, e
			}
		}
		return core.Decoded[T]{Data: out}, nil
	})
}

func (c *Containers) DeleteDomains(ctx context.Context, r types.ResourceRef, in DeleteDomainsOptions, o types.CallOptions) (types.Result[Empty], error) {
	body, _ := json.Marshal(in)
	return call[Empty](ctx, c.client, "container_domains.delete", "/v2/containers/"+url.PathEscape(r.ID)+"/domains", &r, body, o)
}
func (c *Containers) DomainsAndCertificates(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[Domains], error) {
	return call[Domains](ctx, c.client, "container_domains_certificates.get", "/v2/containers/"+url.PathEscape(r.ID)+"/domains/certs", &r, nil, o)
}

func (c *Containers) Options(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ContainerOptions], error) {
	return call[ContainerOptions](ctx, c.client, "container_options.get", "/v2/containers/"+url.PathEscape(r.ID)+"/options", &r, nil, o)
}
func (c *Containers) SetOptions(ctx context.Context, r types.ResourceRef, in SetContainerOptions, o types.CallOptions) (types.Result[ContainerOptions], error) {
	body, _ := json.Marshal(in)
	return call[ContainerOptions](ctx, c.client, "container_options.set", "/v2/containers/"+url.PathEscape(r.ID)+"/options", &r, body, o, in.validate)
}

type LogExports struct{ client *Client }

func (c *LogExports) Create(ctx context.Context, in CreateExportOptions, o types.CallOptions) (types.Result[ExportResponse], error) {
	return c.create(ctx, in, o)
}
func (c *LogExports) Get(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ExportResponse], error) {
	return c.get(ctx, r, o)
}

func (c *LogExports) Cancel(ctx context.Context, r types.ResourceRef, o types.CallOptions) (types.Result[ExportResponse], error) {
	return exportCall(ctx, c.client, "log_exports.cancel", "/v2/logs/"+url.PathEscape(r.ID)+"/cancel", &r, nil, nil, o)
}
