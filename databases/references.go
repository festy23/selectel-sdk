package databases

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

type DatastoreTypes struct {
	runtime *core.Runtime
	scope   types.Scope
}

func referenceOperation(r *core.Runtime, s types.Scope, id string) (core.Operation, error) {
	return r.BindDocumented(core.Operation{ID: id, Service: "databases", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, s)
}
func (c *DatastoreTypes) Iterate(o pagination.Options) (*pagination.Iterator[DatastoreType], error) {
	op, e := referenceOperation(c.runtime, c.scope, "databases.v1.datastore_types.list")
	if e != nil {
		return nil, e
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastore-types"}, struct{}{}, o, "datastore-types", func(v *DatastoreType, _ types.EndpointBinding) error { return v.Validate() })
}
func (c *DatastoreTypes) ListPage(ctx context.Context, o pagination.Options) (pagination.Page[DatastoreType], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Page[DatastoreType]{}, e
	}
	return it.Next(ctx)
}
func (c *DatastoreTypes) Collect(ctx context.Context, o pagination.Options) (pagination.Collection[DatastoreType], error) {
	it, e := c.Iterate(o)
	if e != nil {
		return pagination.Collection[DatastoreType]{Items: []DatastoreType{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}

type validatable interface{ Validate() error }

func referenceGet[T validatable](r *core.Runtime, s types.Scope, operation, path, field, id string) func(context.Context) (types.Result[T], error) {
	return func(ctx context.Context) (types.Result[T], error) {
		if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\?#") {
			return types.Result[T]{}, types.NewError(types.InvalidArgument)
		}
		op, e := referenceOperation(r, s, operation)
		if e != nil {
			return types.Result[T]{}, e
		}
		return core.DoJSON(r, ctx, op, core.Request{Scope: s, Path: path + "/" + url.PathEscape(id)}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[T], error) {
			var envelope map[string]json.RawMessage
			var v T
			if json.Unmarshal(raw, &envelope) != nil || len(envelope[field]) == 0 || string(envelope[field]) == "null" || json.Unmarshal(envelope[field], &v) != nil {
				return core.Decoded[T]{}, types.NewError(types.ProtocolError)
			}
			if e := v.Validate(); e != nil {
				return core.Decoded[T]{}, e
			}
			var returnedID *string
			switch value := any(v).(type) {
			case DatastoreType:
				returnedID = value.ID
			case Flavor:
				returnedID = value.ID
			case ConfigurationParameter:
				returnedID = value.ID
			case UserSettingParameter:
				returnedID = value.ID
			}
			if returnedID != nil && *returnedID != id {
				return core.Decoded[T]{}, types.NewError(types.ScopeMismatch)
			}
			return core.Decoded[T]{Data: v}, nil
		})
	}
}
func (c *DatastoreTypes) Get(ctx context.Context, id string) (types.Result[DatastoreType], error) {
	return referenceGet[DatastoreType](c.runtime, c.scope, "databases.v1.datastore_types.get", "/datastore-types", "datastore-type", id)(ctx)
}

type Flavors struct {
	runtime *core.Runtime
	scope   types.Scope
}
type FlavorListOptions struct {
	ID     *string
	Name   *string
	FlSize *string
}

func (c *Flavors) Iterate(f FlavorListOptions, o pagination.Options) (*pagination.Iterator[Flavor], error) {
	op, e := referenceOperation(c.runtime, c.scope, "databases.v1.flavors.list")
	if e != nil {
		return nil, e
	}
	q := url.Values{}
	for k, v := range map[string]*string{"id": f.ID, "name": f.Name, "fl_size": f.FlSize} {
		if v != nil {
			q.Set(k, *v)
		}
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/flavors", Query: q}, f, o, "flavors", func(v *Flavor, _ types.EndpointBinding) error { return v.Validate() })
}
func (c *Flavors) ListPage(ctx context.Context, f FlavorListOptions, o pagination.Options) (pagination.Page[Flavor], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Flavor]{}, e
	}
	return it.Next(ctx)
}
func (c *Flavors) Collect(ctx context.Context, f FlavorListOptions, o pagination.Options) (pagination.Collection[Flavor], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Flavor]{Items: []Flavor{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Flavors) Get(ctx context.Context, id string) (types.Result[Flavor], error) {
	return referenceGet[Flavor](c.runtime, c.scope, "databases.v1.flavors.get", "/flavors", "flavor", id)(ctx)
}

type ConfigurationParameters struct {
	runtime *core.Runtime
	scope   types.Scope
}
type ConfigurationParameterListOptions struct {
	ID              *string
	Name            *string
	RAM             *string
	VCPUs           *string
	Disk            *string
	DatastoreTypeID *string
}

func (c *ConfigurationParameters) Iterate(f ConfigurationParameterListOptions, o pagination.Options) (*pagination.Iterator[ConfigurationParameter], error) {
	op, e := referenceOperation(c.runtime, c.scope, "databases.v1.configuration_parameters.list")
	if e != nil {
		return nil, e
	}
	q := url.Values{}
	for k, v := range map[string]*string{"id": f.ID, "name": f.Name, "ram": f.RAM, "vcpus": f.VCPUs, "disk": f.Disk, "datastore_type_id": f.DatastoreTypeID} {
		if v != nil {
			q.Set(k, *v)
		}
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/configuration-parameters", Query: q}, f, o, "configuration-parameters", func(v *ConfigurationParameter, _ types.EndpointBinding) error { return v.Validate() })
}
func (c *ConfigurationParameters) ListPage(ctx context.Context, f ConfigurationParameterListOptions, o pagination.Options) (pagination.Page[ConfigurationParameter], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[ConfigurationParameter]{}, e
	}
	return it.Next(ctx)
}
func (c *ConfigurationParameters) Collect(ctx context.Context, f ConfigurationParameterListOptions, o pagination.Options) (pagination.Collection[ConfigurationParameter], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[ConfigurationParameter]{Items: []ConfigurationParameter{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *ConfigurationParameters) Get(ctx context.Context, id string) (types.Result[ConfigurationParameter], error) {
	return referenceGet[ConfigurationParameter](c.runtime, c.scope, "databases.v1.configuration_parameters.get", "/configuration-parameters", "configuration-parameter", id)(ctx)
}

type UserSettingParameters struct {
	runtime *core.Runtime
	scope   types.Scope
}
type UserSettingParameterListOptions struct {
	ID          *string
	Name        *string
	DatastoreID *string

	DatastoreTypeID *string
}

func (c *UserSettingParameters) Iterate(f UserSettingParameterListOptions, o pagination.Options) (*pagination.Iterator[UserSettingParameter], error) {
	op, e := referenceOperation(c.runtime, c.scope, "databases.v1.user_setting_parameters.list")
	if e != nil {
		return nil, e
	}
	q := url.Values{}
	for k, v := range map[string]*string{"id": f.ID, "name": f.Name, "datastore_id": f.DatastoreID, "datastore_type_id": f.DatastoreTypeID} {
		if v != nil {
			q.Set(k, *v)
		}
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/user-setting-parameters", Query: q}, f, o, "user-setting-parameters", func(v *UserSettingParameter, _ types.EndpointBinding) error { return v.Validate() })
}
func (c *UserSettingParameters) ListPage(ctx context.Context, f UserSettingParameterListOptions, o pagination.Options) (pagination.Page[UserSettingParameter], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[UserSettingParameter]{}, e
	}
	return it.Next(ctx)
}
func (c *UserSettingParameters) Collect(ctx context.Context, f UserSettingParameterListOptions, o pagination.Options) (pagination.Collection[UserSettingParameter], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[UserSettingParameter]{Items: []UserSettingParameter{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *UserSettingParameters) Get(ctx context.Context, id string) (types.Result[UserSettingParameter], error) {
	return referenceGet[UserSettingParameter](c.runtime, c.scope, "databases.v1.user_setting_parameters.get", "/user-setting-parameters", "user-setting-parameter", id)(ctx)
}
