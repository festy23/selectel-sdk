package globalrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"reflect"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/internal/endpoints"
	"github.com/festy23/selectel-sdk/types"
)

type DeleteOptions struct{ Cascade, Disconnect *bool }

func deleteResource(c *Client, ctx context.Context, resource, kind string, ref types.ResourceRef, o DeleteOptions, call types.CallOptions) (types.Result[struct{}], error) {
	op, bindErr := c.operation(resource, "delete", "DELETE")
	q := url.Values{}
	for _, v := range []struct {
		key   string
		value *bool
	}{{"cascade", o.Cascade}, {"disconnect", o.Disconnect}} {
		if v.value != nil {
			if *v.value {
				q.Set(v.key, "true")
			} else {
				q.Set(v.key, "false")
			}
		}
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRef(ref, kind)
	}
	decode := func(raw []byte, _ types.EndpointBinding) (out core.Decoded[struct{}], err error) {
		if len(bytes.TrimSpace(raw)) != 0 {
			return out, types.NewError(types.ProtocolError)
		}
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/" + resource + "/" + segment(ref.ID), Query: q, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: call, Validate: validate}, decode)
}
func (c *RoutersClient) Delete(ctx context.Context, ref types.ResourceRef, o DeleteOptions, call types.CallOptions) (types.Result[struct{}], error) {
	return deleteResource(c.client, ctx, "routers", "router", ref, o, call)
}
func (c *NetworksClient) Delete(ctx context.Context, ref types.ResourceRef, o DeleteOptions, call types.CallOptions) (types.Result[struct{}], error) {
	return deleteResource(c.client, ctx, "networks", "network", ref, o, call)
}
func (c *SubnetsClient) Delete(ctx context.Context, ref types.ResourceRef, o DeleteOptions, call types.CallOptions) (types.Result[struct{}], error) {
	return deleteResource(c.client, ctx, "subnets", "subnet", ref, o, call)
}
func (c *StaticRoutesClient) Delete(ctx context.Context, ref types.ResourceRef, call types.CallOptions) (types.Result[struct{}], error) {
	return deleteResource(c.client, ctx, "static_routes", "static_route", ref, DeleteOptions{}, call)
}

func mutate[T any](c *Client, ctx context.Context, resource, kind, method string, spec any, refs []types.ResourceRef, parent *types.ResourceRef, validateInput func() error, call types.CallOptions) (types.Result[T], error) {
	action := "create"
	if method == "PUT" {
		action = "replace"
	}
	op, bindErr := c.operation(resource, action, method)
	body, bodyErr := requestBody(spec)
	path := "/" + resource
	var pinned *types.EndpointBinding
	if method == "PUT" {
		path += "/" + segment(refs[0].ID)
	}
	if len(refs) > 0 {
		pinned = pin(refs[0])
	}
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if bodyErr != nil {
			return bodyErr
		}
		for _, ref := range refs {
			if ref.EndpointID != refs[0].EndpointID || ref.EndpointURL != refs[0].EndpointURL {
				return types.NewError(types.ScopeMismatch)
			}
		}
		if validateInput != nil {
			return validateInput()
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[T], err error) {
		if e := verifyAssociation(raw, spec); e != nil {
			return out, e
		}
		var identity struct {
			ID        *string `json:"id"`
			AccountID *string `json:"account_id"`
			RouterID  *string `json:"router_id"`
			NetworkID *string `json:"network_id"`
			SubnetID  *string `json:"subnet_id"`
		}
		if json.Unmarshal(raw, &identity) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if identity.AccountID != nil && *identity.AccountID != c.scope.AccountID {
			return out, types.NewError(types.ScopeMismatch)
		}
		for ancestor := parent; ancestor != nil; ancestor = ancestor.Parent {
			var actual *string
			switch ancestor.Kind {
			case "router":
				actual = identity.RouterID
			case "network":
				actual = identity.NetworkID
			case "subnet":
				actual = identity.SubnetID
			}
			if actual != nil && *actual != ancestor.ID {
				return out, types.NewError(types.ScopeMismatch)
			}
		}

		id := ""
		if identity.ID != nil {
			id = *identity.ID
		}
		if method == "PUT" {
			if id != "" && id != refs[0].ID {
				return out, types.NewError(types.ScopeMismatch)
			}
			id = refs[0].ID
		}
		if id == "" {
			return out, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(endpoint, "global_router", kind, id)
		if parent != nil {
			copy := cloneRef(*parent)
			ref.Parent = &copy
		}
		out.Refs = []types.ResourceRef{*ref}
		var value T
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := c.bindTree(&value, endpoint, kind, parent); e != nil {
			return out, e
		}
		v := reflect.ValueOf(&value).Elem()
		v.FieldByName("Ref").Set(reflect.ValueOf(ref))
		out.Data = value
		return out, nil
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Body: body, Refs: refs, PinnedEndpoint: pinned, Options: call, Validate: validate}, decode)
}
func (c *RoutersClient) Create(ctx context.Context, spec RouterCreateSpec, call types.CallOptions) (types.Result[Router], error) {
	return mutate[Router](c.client, ctx, "routers", "router", "POST", spec, nil, nil, func() error { return inputError(spec) }, call)
}
func (c *RoutersClient) Update(ctx context.Context, ref types.ResourceRef, spec RouterUpdateSpec, call types.CallOptions) (types.Result[Router], error) {
	return mutate[Router](c.client, ctx, "routers", "router", "PUT", spec, []types.ResourceRef{ref}, ref.Parent, func() error { return c.client.validateRef(ref, "router") }, call)
}

func checkIDs(actual *string, ref types.ResourceRef) error {
	if actual != nil && *actual != ref.ID {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *NetworksClient) Create(ctx context.Context, router, zone types.ResourceRef, spec NetworkCreateSpec, call types.CallOptions) (types.Result[Network], error) {
	first, second := spec.RouterID, spec.ZoneID
	spec.RouterID = &router.ID
	spec.ZoneID = &zone.ID
	return mutate[Network](c.client, ctx, "networks", "network", "POST", spec, []types.ResourceRef{router, zone}, &router, func() error {
		if e := c.client.validateRef(router, "router"); e != nil {
			return e
		}
		if e := c.client.validateRef(zone, "zone"); e != nil {
			return e
		}
		if e := checkIDs(first, router); e != nil {
			return e
		}
		if e := checkIDs(second, zone); e != nil {
			return e
		}
		return inputError(spec)
	}, call)
}
func (c *SubnetsClient) Create(ctx context.Context, network types.ResourceRef, spec SubnetCreateSpec, call types.CallOptions) (types.Result[Subnet], error) {
	actual := spec.NetworkID
	spec.NetworkID = &network.ID
	return mutate[Subnet](c.client, ctx, "subnets", "subnet", "POST", spec, []types.ResourceRef{network}, &network, func() error {
		if e := c.client.validateRef(network, "network"); e != nil {
			return e
		}
		if e := checkIDs(actual, network); e != nil {
			return e
		}
		return inputError(spec)
	}, call)
}
func (c *StaticRoutesClient) Create(ctx context.Context, router types.ResourceRef, spec StaticRouteCreateSpec, call types.CallOptions) (types.Result[StaticRoute], error) {
	actual := spec.RouterID
	spec.RouterID = &router.ID
	return mutate[StaticRoute](c.client, ctx, "static_routes", "static_route", "POST", spec, []types.ResourceRef{router}, &router, func() error {
		if e := c.client.validateRef(router, "router"); e != nil {
			return e
		}
		if e := checkIDs(actual, router); e != nil {
			return e
		}
		return inputError(spec)
	}, call)
}
func (c *NetworksClient) Update(ctx context.Context, ref types.ResourceRef, spec NetworkUpdateSpec, call types.CallOptions) (types.Result[Network], error) {
	return mutate[Network](c.client, ctx, "networks", "network", "PUT", spec, []types.ResourceRef{ref}, ref.Parent, func() error { return c.client.validateRef(ref, "network") }, call)
}
func (c *SubnetsClient) Update(ctx context.Context, ref types.ResourceRef, spec SubnetUpdateSpec, call types.CallOptions) (types.Result[Subnet], error) {
	return mutate[Subnet](c.client, ctx, "subnets", "subnet", "PUT", spec, []types.ResourceRef{ref}, ref.Parent, func() error { return c.client.validateRef(ref, "subnet") }, call)
}
func (c *StaticRoutesClient) Update(ctx context.Context, ref types.ResourceRef, spec StaticRouteUpdateSpec, call types.CallOptions) (types.Result[StaticRoute], error) {
	return mutate[StaticRoute](c.client, ctx, "static_routes", "static_route", "PUT", spec, []types.ResourceRef{ref}, ref.Parent, func() error { return c.client.validateRef(ref, "static_route") }, call)
}

// AttachVPC preserves the caller's original network scope. It attaches that
// exact resource; the new Global Router identity is not a Neutron network.
func (c *NetworksClient) AttachVPC(ctx context.Context, router, zone, origin types.ResourceRef, spec NetworkCreateSpec, call types.CallOptions) (types.Result[Network], error) {
	wire := SourceVPCNetworkCreateSpec{Name: spec.Name, Tags: spec.Tags, RouterID: &router.ID, ZoneID: &zone.ID, ProjectID: &origin.Scope.ProjectID, OsNetworkID: &origin.ID}
	validate := func() error {
		if e := validateOrigin(c.client, origin, "network"); e != nil {
			return e
		}
		if e := c.client.validateRef(router, "router"); e != nil {
			return e
		}
		if e := c.client.validateRef(zone, "zone"); e != nil {
			return e
		}
		if e := checkIDs(spec.RouterID, router); e != nil {
			return e
		}
		if e := checkIDs(spec.ZoneID, zone); e != nil {
			return e
		}
		if e := checkIDs(spec.OsNetworkID, origin); e != nil {
			return e
		}
		if spec.Vlan != nil || spec.InnerVlan != nil || spec.VdcName != nil {
			return types.NewError(types.InvalidArgument)
		}
		return inputError(wire)
	}
	out, err := mutate[Network](c.client, ctx, "networks", "network", "POST", wire, []types.ResourceRef{router, zone}, &router, validate, call)
	if out.Receipt != nil && validateOrigin(c.client, origin, "network") == nil {
		out.Receipt.ResourceRefs = append(out.Receipt.ResourceRefs, cloneRef(origin))
	}
	return out, err
}
func (c *SubnetsClient) AttachVPC(ctx context.Context, network, origin types.ResourceRef, spec SubnetCreateSpec, call types.CallOptions) (types.Result[Subnet], error) {
	wire := SourceVPCSubnetCreateSpec{Name: spec.Name, Tags: spec.Tags, NetworkID: &network.ID, ProjectID: &origin.Scope.ProjectID, OsSubnetID: &origin.ID, Cidr: spec.Cidr, Gateway: spec.Gateway, ServiceAddresses: spec.ServiceAddresses}
	validate := func() error {
		if e := validateOrigin(c.client, origin, "subnet"); e != nil {
			return e
		}
		if e := c.client.validateRef(network, "network"); e != nil {
			return e
		}
		if e := checkIDs(spec.NetworkID, network); e != nil {
			return e
		}
		if e := checkIDs(spec.OsSubnetID, origin); e != nil {
			return e
		}
		return inputError(wire)
	}
	out, err := mutate[Subnet](c.client, ctx, "subnets", "subnet", "POST", wire, []types.ResourceRef{network}, &network, validate, call)
	if out.Receipt != nil && validateOrigin(c.client, origin, "subnet") == nil {
		out.Receipt.ResourceRefs = append(out.Receipt.ResourceRefs, cloneRef(origin))
	}
	return out, err
}
func validateOrigin(c *Client, origin types.ResourceRef, kind string) error {
	if e := origin.Validate(); e != nil {
		return e
	}
	if origin.Kind != kind || origin.Service != "network" || origin.APIGeneration != "v2" || origin.Scope.ProjectID == "" || origin.Scope.Region == "" || origin.Scope.Pool == "" {
		return types.NewError(types.InvalidArgument)
	}
	if origin.Scope.AccountID != c.scope.AccountID {
		return types.NewError(types.ScopeMismatch)
	}
	if _, e := endpoints.Location(origin.Scope.Region, origin.Scope.Pool); e != nil {
		return e
	}
	for ancestor := origin.Parent; ancestor != nil; ancestor = ancestor.Parent {
		if ancestor.Service != origin.Service || ancestor.APIGeneration != origin.APIGeneration || ancestor.EndpointID != origin.EndpointID || ancestor.EndpointURL != origin.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}

// verifyAssociation checks caller-selected source identities before retaining any new ID.
func verifyAssociation(raw []byte, spec any) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return types.NewError(types.ProtocolError)
	}
	v := reflect.ValueOf(spec)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	for _, pair := range []struct{ field, wire string }{{"ProjectID", "project_id"}, {"OsNetworkID", "os_network_id"}, {"OsSubnetID", "os_subnet_id"}, {"ZoneID", "zone_id"}} {
		field := v.FieldByName(pair.field)
		if !field.IsValid() || field.IsNil() {
			continue
		}
		actual, present := fields[pair.wire]
		if !present {
			continue
		}
		var value *string
		if json.Unmarshal(actual, &value) != nil {
			return types.NewError(types.ProtocolError)
		}
		if value == nil || *value != field.Elem().String() {
			return types.NewError(types.ScopeMismatch)
		}
	}
	return nil
}
