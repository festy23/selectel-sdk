// Package globalrouter binds the published account-scoped Global Router V1 API.
package globalrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	headers      http.Header
	runtime      *core.Runtime
	scope        types.Scope
	Routers      *RoutersClient
	Networks     *NetworksClient
	Subnets      *SubnetsClient
	StaticRoutes *StaticRoutesClient
}
type RoutersClient struct{ client *Client }
type NetworksClient struct{ client *Client }
type SubnetsClient struct{ client *Client }
type StaticRoutesClient struct{ client *Client }

func Bind(r *core.Runtime, s types.Scope) *Client {
	c := &Client{runtime: r, scope: s}
	c.Routers = &RoutersClient{c}
	c.Networks = &NetworksClient{c}
	c.Subnets = &SubnetsClient{c}
	c.StaticRoutes = &StaticRoutesClient{c}
	return c
}

type GetOptions struct{ Nested *bool }

func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
func (c *Client) operation(resource, action, method string) (core.Operation, error) {
	op := core.Operation{ID: "global_router.v1." + resource + "." + action, Service: "global_router", APIGeneration: "v1", Method: method, Effect: "read", Auth: types.IAMAccount, Success: []int{200}}
	if method != "GET" {
		op.Effect = "mutation"
		op.Rejected = []int{400, 403, 404}
		if method == "POST" {
			op.Success = []int{202}
			op.Rejected = []int{400, 403}
		}
		if method == "DELETE" {
			op.Success = []int{204}
		}
	}
	op.Headers = c.headers.Clone()
	return c.runtime.BindDocumented(op, c.scope)
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
}
func (c *Client) validateRef(ref types.ResourceRef, kind string) error {
	if e := ref.Validate(); e != nil {
		return e
	}
	if ref.Kind != kind {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "global_router" || ref.APIGeneration != "v1" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	for current := &ref; current.Parent != nil; current = current.Parent {
		parent := current.Parent
		if parent.Service != ref.Service || parent.APIGeneration != ref.APIGeneration || parent.EndpointID != ref.EndpointID || parent.EndpointURL != ref.EndpointURL || parent.Scope != ref.Scope {
			return types.NewError(types.ScopeMismatch)
		}
		valid := current.Kind == "network" && parent.Kind == "router" || current.Kind == "subnet" && parent.Kind == "network" || current.Kind == "static_route" && (parent.Kind == "router" || parent.Kind == "subnet")
		if !valid {
			return types.NewError(types.InvalidArgument)
		}
	}

	return nil
}

// bindTree validates account identity and binds only this service's explicit IDs.
// IDs belonging to Neutron or another service remain metadata, never adopted refs.
func (c *Client) bindTree(value any, endpoint types.EndpointBinding, kind string, parent *types.ResourceRef) error {
	v := reflect.ValueOf(value).Elem()
	account := v.FieldByName("AccountID")
	if account.IsValid() && !account.IsNil() && account.Elem().String() != c.scope.AccountID {
		return types.NewError(types.ScopeMismatch)
	}
	if parent == nil {
		key, parentKind := "", ""
		switch kind {
		case "network":
			key, parentKind = "RouterID", "router"
		case "subnet":
			key, parentKind = "NetworkID", "network"
		case "static_route":
			key, parentKind = "RouterID", "router"
		}
		if key != "" {
			field := v.FieldByName(key)
			if field.IsValid() && !field.IsNil() && field.Elem().String() != "" {
				parent = core.Ref(endpoint, "global_router", parentKind, field.Elem().String())
			}
		}
	}
	id := v.FieldByName("ID")
	var ref *types.ResourceRef
	if id.IsValid() && !id.IsNil() && id.Elem().String() != "" {
		ref = core.Ref(endpoint, "global_router", kind, id.Elem().String())
		if parent != nil {
			copy := cloneRef(*parent)
			ref.Parent = &copy
		}
		field := v.FieldByName("Ref")
		if field.IsValid() {
			field.Set(reflect.ValueOf(ref))
		}
	}
	for current := parent; current != nil; current = current.Parent {
		key := "RouterID"
		if current.Kind == "network" {
			key = "NetworkID"
		}
		if current.Kind == "subnet" {
			key = "SubnetID"
		}
		field := v.FieldByName(key)
		if field.IsValid() && !field.IsNil() && field.Elem().String() != current.ID {
			return types.NewError(types.ScopeMismatch)
		}
	}

	for _, nested := range []struct{ field, kind string }{{"Networks", "network"}, {"Subnets", "subnet"}, {"StaticRoutes", "static_route"}} {
		items := v.FieldByName(nested.field)
		if !items.IsValid() {
			continue
		}
		for i := 0; i < items.Len(); i++ {
			if ref == nil {
				return types.NewError(types.ProtocolError)
			}
			if e := c.bindTree(items.Index(i).Addr().Interface(), endpoint, nested.kind, ref); e != nil {
				return e
			}
		}
	}
	return nil
}
func get[T any](c *Client, ctx context.Context, resource, kind string, ref types.ResourceRef, options GetOptions, call types.CallOptions) (types.Result[T], error) {
	op, bindErr := c.operation(resource, "get", "GET")
	q := url.Values{}
	if options.Nested != nil {
		if *options.Nested {
			q.Set("nested", "true")
		} else {
			q.Set("nested", "false")
		}
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if resource != "routers" && options.Nested != nil {
			return types.NewError(types.InvalidArgument)
		}
		return c.validateRef(ref, kind)
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[T], err error) {
		var value T
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := c.bindTree(&value, endpoint, kind, ref.Parent); e != nil {
			return out, e
		}
		v := reflect.ValueOf(value)
		id := v.FieldByName("ID")
		if id.IsNil() || id.Elem().String() != ref.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		out.Data = value
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/" + resource + "/" + segment(ref.ID), Query: q, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: call, Validate: validate}, decode)
}
func (c *RoutersClient) Get(ctx context.Context, ref types.ResourceRef, o GetOptions, call types.CallOptions) (types.Result[Router], error) {
	return get[Router](c.client, ctx, "routers", "router", ref, o, call)
}
func (c *NetworksClient) Get(ctx context.Context, ref types.ResourceRef, o GetOptions, call types.CallOptions) (types.Result[Network], error) {
	return get[Network](c.client, ctx, "networks", "network", ref, o, call)
}
func (c *SubnetsClient) Get(ctx context.Context, ref types.ResourceRef, o GetOptions, call types.CallOptions) (types.Result[Subnet], error) {
	return get[Subnet](c.client, ctx, "subnets", "subnet", ref, o, call)
}
func (c *StaticRoutesClient) Get(ctx context.Context, ref types.ResourceRef, call types.CallOptions) (types.Result[StaticRoute], error) {
	return get[StaticRoute](c.client, ctx, "static_routes", "static_route", ref, GetOptions{}, call)
}

func cloneRef(ref types.ResourceRef) types.ResourceRef {
	if ref.Parent != nil {
		parent := cloneRef(*ref.Parent)
		ref.Parent = &parent
	}
	return ref
}
