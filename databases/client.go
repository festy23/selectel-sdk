// Package databases exposes explicit generations of managed database APIs.
package databases

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type Client struct{ V1 V1Client }
type V1Client struct{ Datastores *Datastores }
type Datastores struct {
	runtime *core.Runtime
	scope   types.Scope
}
type ListOptions struct {
	ID           *string `json:"id,omitempty"`
	Name         *string `json:"name,omitempty"`
	Status       *string `json:"status,omitempty"`
	Enabled      *string `json:"enabled,omitempty"`
	TypeID       *string `json:"type_id,omitempty"`
	FlavorID     *string `json:"flavor_id,omitempty"`
	SubnetID     *string `json:"subnet_id,omitempty"`
	AllowRestore *string `json:"allow_restore,omitempty"`
	Maintenance  *string `json:"is_maintenance,omitempty"`
	Protected    *string `json:"is_protected,omitempty"`
	Deleted      *string `json:"deleted,omitempty"`
}

func Bind(r *core.Runtime, scope types.Scope) *Client {
	return &Client{V1: V1Client{Datastores: &Datastores{runtime: r, scope: scope}}}
}
func (c *Datastores) Iterate(f ListOptions, o pagination.Options) (*pagination.Iterator[Datastore], error) {
	op, e := c.runtime.BindDocumented(core.Operation{ID: "databases.v1.datastores.list", Service: "databases", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, c.scope)
	if e != nil {
		return nil, e
	}
	query := url.Values{"project_id": {c.scope.ProjectID}}
	for name, value := range map[string]*string{"id": f.ID, "name": f.Name, "status": f.Status, "enabled": f.Enabled, "type_id": f.TypeID, "flavor_id": f.FlavorID, "subnet_id": f.SubnetID, "allow_restore": f.AllowRestore, "is_maintenance": f.Maintenance, "is_protected": f.Protected, "deleted": f.Deleted} {
		if value != nil {
			query.Set(name, *value)
		}
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/datastores", Query: query}, f, o, "datastores", func(v *Datastore, endpoint types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ProjectID != nil && *v.ProjectID != endpoint.Scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "databases", "datastore", *v.ID)
		}
		return nil
	})
}
func (c *Datastores) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[Datastore], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Datastore]{}, e
	}
	return it.Next(ctx)
}
func (c *Datastores) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[Datastore], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Datastore]{Items: []Datastore{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
