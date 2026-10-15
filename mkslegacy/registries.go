package mkslegacy

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) GetRegistryIntegrations(ctx context.Context, cluster types.ResourceRef, options types.CallOptions) (types.Result[RegistriesIntegration], error) {
	op := core.Operation{ID: "mks.v1.clusters_registries.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(cluster) + "/registries", Refs: []types.ResourceRef{cluster}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateRoot(cluster, "cluster", op)
	}}, registryDecoder(cluster))
}
func registryDecoder(cluster types.ResourceRef) core.Decoder[RegistriesIntegration] {
	return func(raw []byte, ep types.EndpointBinding) (core.Decoded[RegistriesIntegration], error) {
		d := core.Decoded[RegistriesIntegration]{}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil || envelope == nil {
			return d, types.NewError(types.ProtocolError)
		}
		var rows []json.RawMessage
		if field, ok := envelope["registries"]; ok {
			if json.Unmarshal(field, &rows) != nil || rows == nil {
				return d, types.NewError(types.ProtocolError)
			}
		}
		seen := map[string]bool{}
		var firstErr error
		for _, row := range rows {
			var identity struct {
				ID *string `json:"id"`
			}
			if json.Unmarshal(row, &identity) != nil || identity.ID == nil || *identity.ID == "" || seen[*identity.ID] {
				if firstErr == nil {
					firstErr = types.NewError(types.ProtocolError)
				}
				continue
			}
			seen[*identity.ID] = true
			ref := core.Ref(ep, "mks", "registry_integration", *identity.ID)
			parent := cloneRef(cluster)
			ref.Parent = &parent
			d.Refs = append(d.Refs, *ref)
		}
		if firstErr != nil {
			return d, firstErr
		}
		if json.Unmarshal(raw, &d.Data) != nil || d.Data.Validate() != nil {
			return d, types.NewError(types.ProtocolError)
		}
		for i := range d.Data.Registries {
			ref := core.Ref(ep, "mks", "registry_integration", *d.Data.Registries[i].ID)
			parent := cloneRef(cluster)
			ref.Parent = &parent
			d.Data.Registries[i].Ref = ref
		}
		return d, nil
	}
}

func (c *Client) CreateRegistryIntegrations(ctx context.Context, cluster types.ResourceRef, input RegistryCreateSpec, options types.CallOptions) (types.Result[RegistriesIntegration], error) {
	body, e := requestBody("", input)
	if input.Validate() != nil {
		e = types.NewError(types.InvalidArgument)
	}
	for _, v := range input.Registries {
		if v.ID == nil || *v.ID == "" {
			e = types.NewError(types.InvalidArgument)
		}
	}
	return mutateRegistries(c, ctx, "mks.v1.clusters_registries.create", "POST", 201, cluster, body, e, options)
}
func mutateRegistries(c *Client, ctx context.Context, id, method string, status int, cluster types.ResourceRef, body []byte, inputErr error, options types.CallOptions) (types.Result[RegistriesIntegration], error) {
	op := core.Operation{ID: id, Service: "mks", APIGeneration: "v1", Method: method, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{status}, Rejected: []int{404}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(cluster) + "/registries", Body: body, Refs: []types.ResourceRef{cluster}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRoot(cluster, "cluster", op); e != nil {
			return e
		}
		return inputErr
	}}, registryDecoder(cluster))
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

func (c *Client) ReplaceRegistryIntegrations(ctx context.Context, cluster types.ResourceRef, input RegistryUpdateSpec, options types.CallOptions) (types.Result[RegistriesIntegration], error) {
	body, e := requestBody("", input)
	if input.Validate() != nil {
		e = types.NewError(types.InvalidArgument)
	}
	for _, v := range input.Registries {
		if v.ID == nil || *v.ID == "" {
			e = types.NewError(types.InvalidArgument)
		}
	}
	return mutateRegistries(c, ctx, "mks.v1.clusters_registries.replace", "PUT", 200, cluster, body, e, options)
}

func (c *Client) DeleteAllRegistryIntegrations(ctx context.Context, cluster types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.clusters_registries.delete", "DELETE", resourcePath(cluster)+"/registries", []types.ResourceRef{cluster}, nil, options, func(op core.Operation) error { return c.validateRoot(cluster, "cluster", op) })
}

func (c *Client) DeleteRegistryIntegration(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.registry_integrations.delete", "DELETE", resourcePath(ref), []types.ResourceRef{ref}, nil, options, func(op core.Operation) error { return c.validateNested(ref, "registry_integration", op) })
}
