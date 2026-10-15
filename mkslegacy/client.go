// Package mkslegacy binds explicit MKS v1 resources without converting v2 refs.
package mkslegacy

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{r, scope} }
func (c *Client) ListClusters(ctx context.Context, options pagination.Options) (pagination.Collection[Cluster], error) {
	op := core.Operation{ID: "mks.v1.clusters.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[Cluster]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters"}, struct{}{}, options, "clusters", func(v *Cluster, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" || v.ProjectID == nil || v.Region == nil {
			return types.NewError(types.ProtocolError)
		}
		if *v.ProjectID != c.scope.ProjectID || *v.Region != c.scope.Pool {
			return types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(ep, "mks", "cluster", *v.ID)
		return nil
	})
	if e != nil {
		return pagination.Collection[Cluster]{}, e
	}
	return iterator.Collect(ctx)
}
