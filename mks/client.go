// Package mks exposes explicit generations of Managed Kubernetes APIs.
package mks

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct{ V2 V2Client }
type V2Client struct {
	Clusters      *Clusters
	Nodegroups    *Nodegroups
	Nodes         *Nodes
	Tasks         *Tasks
	Registries    *RegistryIntegrations
	Configuration *Configuration
}
type Clusters struct {
	runtime *core.Runtime
	scope   types.Scope
}
type ListOptions struct{}

func Bind(r *core.Runtime, scope types.Scope) *Client {
	s := service{runtime: r, scope: scope}
	return &Client{V2: V2Client{Clusters: &Clusters{runtime: r, scope: scope}, Nodegroups: &Nodegroups{service: s}, Nodes: &Nodes{service: s}, Tasks: &Tasks{service: s}, Registries: &RegistryIntegrations{service: s}, Configuration: &Configuration{service: s}}}
}
func (c *Clusters) Iterate(f ListOptions, o pagination.Options) (*pagination.Iterator[Cluster], error) {
	op, e := c.runtime.BindDocumented(core.Operation{ID: "mks.v2.clusters.list", Service: "mks", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, c.scope)
	if e != nil {
		return nil, e
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters"}, f, o, "clusters", func(v *Cluster, endpoint types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ProjectID != nil && *v.ProjectID != endpoint.Scope.ProjectID {
			return types.NewError(types.ScopeMismatch)
		}
		if v.Pool != nil && *v.Pool != endpoint.Scope.Pool {
			return types.NewError(types.ScopeMismatch)
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "mks", "cluster", *v.ID)
		}
		return nil
	})
}
func (c *Clusters) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[Cluster], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Cluster]{}, e
	}
	return it.Next(ctx)
}
func (c *Clusters) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[Cluster], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Cluster]{Items: []Cluster{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
