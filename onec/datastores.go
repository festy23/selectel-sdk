package onec

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
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
		if bindErr != nil {
			return bindErr
		}
		if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.EndpointUnavailable)
		}
		return nil
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
		if bindErr != nil {
			return bindErr
		}
		if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.EndpointUnavailable)
		}
		return nil
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

func (c *Client) PatchDatastore(ctx context.Context, ref types.ResourceRef, spec DatastoreSpec, options types.CallOptions) (types.Result[PatchDatastoreResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastores.update", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPatch, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
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
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if bindErr != nil {
			return bindErr
		}
		if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.EndpointUnavailable)
		}
		return nil
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + url.PathEscape(ref.ID), Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[PatchDatastoreResponse], error) {
		var identity struct {
			Datastore *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"datastore"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Datastore == nil || identity.Datastore.ID == nil {
			return core.Decoded[PatchDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Datastore.ID != ref.ID || identity.Datastore.ProjectID != nil && *identity.Datastore.ProjectID != c.scope.ProjectID {
			return core.Decoded[PatchDatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[PatchDatastoreResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Datastore == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Datastore.Ref = core.Ref(ep, "cloud_1c", "datastore", ref.ID)
		return out, nil
	})
}

func (c *Client) ResizeDatastore(ctx context.Context, ref types.ResourceRef, spec ResizeDatastoreBody, options types.CallOptions) (types.Result[ResizeDatastoreResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastores_resize.create", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := json.Marshal(spec)
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
		if spec.Validate() != nil || bodyErr != nil {
			return types.NewError(types.InvalidArgument)
		}
		if bindErr != nil {
			return bindErr
		}
		if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
			return types.NewError(types.EndpointUnavailable)
		}
		return nil
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + url.PathEscape(ref.ID) + "/resize", Body: body, Options: options, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint, Validate: validate}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ResizeDatastoreResponse], error) {
		var identity struct {
			Datastore *struct {
				ID        *string `json:"id"`
				ProjectID *string `json:"projectId"`
			} `json:"datastore"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.Datastore == nil || identity.Datastore.ID == nil {
			return core.Decoded[ResizeDatastoreResponse]{}, types.NewError(types.ProtocolError)
		}
		if *identity.Datastore.ID != ref.ID || identity.Datastore.ProjectID != nil && *identity.Datastore.ProjectID != c.scope.ProjectID {
			return core.Decoded[ResizeDatastoreResponse]{}, types.NewError(types.ScopeMismatch)
		}
		out := core.Decoded[ResizeDatastoreResponse]{Refs: []types.ResourceRef{ref}}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Validate() != nil || out.Data.Datastore == nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Datastore.Ref = core.Ref(ep, "cloud_1c", "datastore", ref.ID)
		return out, nil
	})
}

func (c *Client) ListDatastoreFlavors(ctx context.Context, includeDeprecated *bool, options pagination.Options) (pagination.Collection[DatastoreFlavor], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastore_flavors.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return pagination.Collection[DatastoreFlavor]{}, err
	}
	query := url.Values{}
	if includeDeprecated != nil {
		query.Set("includeDeprecated", strconv.FormatBool(*includeDeprecated))
	}
	iterator, err := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastore-flavors", Query: query}, query.Encode(), options, "flavors", func(v *DatastoreFlavor, _ types.EndpointBinding) error { return v.Validate() })
	if err != nil {
		return pagination.Collection[DatastoreFlavor]{}, err
	}
	return iterator.Collect(ctx)
}

func (c *Client) GetDatastoreFlavorConstraints(ctx context.Context, options types.CallOptions) (types.Result[GetDatastoreFlavorConstraintsResponse], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastore_flavors_constraints.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastore-flavors/constraints", Options: options, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetDatastoreFlavorConstraintsResponse], error) {
		var v GetDatastoreFlavorConstraintsResponse
		if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Constraints == nil {
			return core.Decoded[GetDatastoreFlavorConstraintsResponse]{}, types.NewError(types.ProtocolError)
		}
		return core.Decoded[GetDatastoreFlavorConstraintsResponse]{Data: v}, nil
	})
}

func (c *Client) ListDatastoreTypes(ctx context.Context, options pagination.Options) (pagination.Collection[DatastoreType], error) {
	op := core.Operation{ID: "cloud_1c.v2.datastore_types.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return pagination.Collection[DatastoreType]{}, err
	}
	query := url.Values{}
	iterator, err := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastore-types", Query: query}, query.Encode(), options, "types", func(v *DatastoreType, _ types.EndpointBinding) error { return v.Validate() })
	if err != nil {
		return pagination.Collection[DatastoreType]{}, err
	}
	return iterator.Collect(ctx)
}

type DatastoreListing struct {
	pagination.Collection[Datastore]
	Envelopes []ListDatastoresResponse `json:"envelopes,omitempty"`
}

// ListDatastores preserves bounded pages and provider cursor metadata. The pinned
// source has no terminal sentinel contract; it never infers completion from 0,
// an omitted cursor or an empty page.
func (c *Client) ListDatastores(ctx context.Context, options pagination.Options) (DatastoreListing, error) {
	result := DatastoreListing{}
	op := core.Operation{ID: "cloud_1c.v2.datastores.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return result, err
	}
	plan, err := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, nil)
	if err != nil {
		return result, err
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[Datastore], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, err := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores", Query: query}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListDatastoresResponse], error) {
			var v ListDatastoresResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Datastores == nil {
				return core.Decoded[ListDatastoresResponse]{}, types.NewError(types.ProtocolError)
			}
			if v.Cursor != nil {
				for _, field := range []*string{v.Cursor.Next, v.Cursor.Total} {
					if field != nil {
						if _, e := strconv.ParseUint(*field, 10, 64); e != nil {
							return core.Decoded[ListDatastoresResponse]{}, types.NewError(types.ProtocolError)
						}
					}
				}
			}
			seen := map[string]bool{}
			for i := range v.Datastores {
				row := &v.Datastores[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListDatastoresResponse]{}, types.NewError(types.ProtocolError)
				}
				if row.ProjectID != nil && *row.ProjectID != c.scope.ProjectID {
					return core.Decoded[ListDatastoresResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "datastore", *row.ID)
			}
			return core.Decoded[ListDatastoresResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[Datastore]{Metadata: out.Metadata, Next: input}
		if err != nil {
			return page, err
		}
		page.Items = out.Data.Datastores
		out.Data.Datastores = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	iterator, err := pagination.New(plan, options, fetch)
	if err != nil {
		return result, err
	}
	result.Collection, err = iterator.Collect(ctx)
	return result, err
}

type DatastoreMaintenanceListing struct {
	pagination.Collection[DatastoreMntEvent]
	Envelopes []ListDatastoreMntEventsResponse `json:"envelopes,omitempty"`
}

func (c *Client) ListDatastoreMaintenanceEvents(ctx context.Context, ref types.ResourceRef, options pagination.Options) (DatastoreMaintenanceListing, error) {
	result := DatastoreMaintenanceListing{}
	if e := ref.Validate(); e != nil {
		return result, e
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return result, types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "datastore" || ref.Parent != nil {
		return result, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{ID: "cloud_1c.v2.datastores_maintenance.list", Service: "cloud_1c", APIGeneration: "v2", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return result, e
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return result, types.NewError(types.EndpointUnavailable)
	}
	endpoint := types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}
	plan, e := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, ref)
	if e != nil {
		return result, e
	}
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[DatastoreMntEvent], error) {
		query := url.Values{"cursor.limit": {strconv.FormatUint(uint64(size), 10)}}
		if input != "" {
			query.Set("cursor.cursor", input)
		}
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/datastores/" + url.PathEscape(ref.ID) + "/maintenance", Query: query, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &endpoint}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[ListDatastoreMntEventsResponse], error) {
			var v ListDatastoreMntEventsResponse
			if json.Unmarshal(raw, &v) != nil || v.Validate() != nil || v.Events == nil {
				return core.Decoded[ListDatastoreMntEventsResponse]{}, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range v.Events {
				row := &v.Events[i]
				if row.ID == nil || *row.ID == "" || seen[*row.ID] {
					return core.Decoded[ListDatastoreMntEventsResponse]{}, types.NewError(types.ProtocolError)
				}
				if row.ProjectID != nil && *row.ProjectID != c.scope.ProjectID || row.Spec != nil && row.Spec.DatastoreID != nil && *row.Spec.DatastoreID != ref.ID {
					return core.Decoded[ListDatastoreMntEventsResponse]{}, types.NewError(types.ScopeMismatch)
				}
				seen[*row.ID] = true
				row.Ref = core.Ref(ep, "cloud_1c", "datastore_maintenance_event", *row.ID)
				parent := ref
				row.Ref.Parent = &parent
			}
			return core.Decoded[ListDatastoreMntEventsResponse]{Data: v}, nil
		})
		page := pagination.SourcePage[DatastoreMntEvent]{Metadata: out.Metadata, Next: input}
		if e != nil {
			return page, e
		}
		page.Items = out.Data.Events
		out.Data.Events = nil
		if out.Data.Cursor != nil && out.Data.Cursor.Next != nil {
			page.Next = *out.Data.Cursor.Next
		}
		result.Envelopes = append(result.Envelopes, out.Data)
		return page, nil
	}
	iterator, e := pagination.New(plan, options, fetch)
	if e != nil {
		return result, e
	}
	result.Collection, e = iterator.Collect(ctx)
	return result, e
}
