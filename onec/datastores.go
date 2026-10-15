package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

func (c *Client) GetDatastore(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[GetDatastoreResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastores.get", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "datastore" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[GetDatastoreResponse], error) {
		var identity struct {
			Datastore *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"datastore"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Datastore == nil || identity.Datastore.ID == nil {
			return core.Decoded[GetDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Datastore.ID != ref.ID || (identity.Datastore.ProjectID != nil && *identity.Datastore.ProjectID != c.scope.ProjectID) {
			return core.Decoded[GetDatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		var v GetDatastoreResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Datastore == nil {
			return core.Decoded[GetDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		v.Datastore.Ref = core.Ref(endpoint, "cloud_1c", "datastore", ref.ID)
		return core.Decoded[GetDatastoreResponse]{Data: v, Refs: []types.ResourceRef{*v.Datastore.Ref}}, nil
	})
}

func (c *Client) CreateDatastore(ctx context.Context, spec CreateDatastoreRequest, options types.CallOptions) (types.Result[CreateDatastoreResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastores.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores", Body: body, Options: options, Validate: func() error {
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[CreateDatastoreResponse], error) {
		var identity struct {
			Datastore *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"datastore"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Datastore == nil || identity.Datastore.ID == nil || *identity.Datastore.ID == "" {
			return core.Decoded[CreateDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if identity.Datastore.ProjectID != nil && *identity.Datastore.ProjectID != c.scope.ProjectID {
			return core.Decoded[CreateDatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(endpoint, "cloud_1c", "datastore", *identity.Datastore.ID)
		out := core.Decoded[CreateDatastoreResponse]{Refs: []types.ResourceRef{*ref}}
		var v CreateDatastoreResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Datastore == nil {
			return out, types.NewError(types.ProtocolError)
		}
		v.Datastore.Ref = ref
		out.Data = v
		return out, nil
	})
}

func (c *Client) DeleteDatastore(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DeleteDatastoreResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastores.delete", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodDelete, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	validate := func() error {
		if err := ref.Validate(); err != nil {
			return err
		}
		if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
			return types.NewError(types.ScopeMismatch)
		}
		if ref.Kind != "datastore" || ref.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
		return bindErr
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + url.PathEscape(ref.ID), Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DeleteDatastoreResponse], error) {
		var identity struct {
			Datastore *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"datastore"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Datastore == nil || identity.Datastore.ID == nil {
			return core.Decoded[DeleteDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Datastore.ID != ref.ID || identity.Datastore.ProjectID != nil && *identity.Datastore.ProjectID != c.scope.ProjectID {
			return core.Decoded[DeleteDatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[DeleteDatastoreResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Datastore == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Datastore.Ref = core.Ref(ep, "cloud_1c", "datastore", ref.ID)
		return out, nil
	})
}
