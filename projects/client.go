// Package projects manages account-scoped Resell v2 projects.
package projects

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strconv"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}
type ListOptions struct {
	All *bool `json:"all,omitempty"`
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{runtime: r, scope: scope} }
func (c *Client) Iterate(filters ListOptions, options pagination.Options) (*pagination.Iterator[Project], error) {
	if c.scope.ProjectID != "" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	op := core.Operation{ID: "projects.v2.projects.list", Service: "projects", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMAccount, EndpointID: "selectel-resell", FixedEndpoint: "https://api.selectel.ru/vpc/resell", Success: []int{200}}
	query := url.Values{}
	if filters.All != nil {
		query.Set("all", strconv.FormatBool(*filters.All))
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/v2/projects", Query: query}, filters, options, "projects", func(v *Project, endpoint types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != nil {
			v.Ref = core.Ref(endpoint, "projects", "project", *v.ID)
		}
		return nil
	})
}
func (c *Client) ListPage(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Page[Project], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Page[Project]{}, e
	}
	return it.Next(ctx)
}
func (c *Client) Collect(ctx context.Context, f ListOptions, o pagination.Options) (pagination.Collection[Project], error) {
	it, e := c.Iterate(f, o)
	if e != nil {
		return pagination.Collection[Project]{Items: []Project{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
