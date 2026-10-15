// Package mobilefarm implements the documented Mobile Farm v3 and auth v2 APIs.
package mobilefarm

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(runtime *core.Runtime, scope types.Scope) *Client {
	return &Client{runtime: runtime, scope: scope}
}

func (c *Client) AvailableDevices(ctx context.Context, fields []string, options pagination.Options) (pagination.Collection[AvailableDevice], error) {
	op := core.Operation{ID: "mobile_farm.v3.api_devices_available.list", Service: "mobile_farm", APIGeneration: "v3", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, Success: []int{200}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 401: types.Unauthenticated, 403: types.PermissionDenied}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[AvailableDevice]{}, e
	}
	query := url.Values{}
	for _, field := range fields {
		query.Add("fields", field)
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/api/v3/devices/available", Query: query}, fields, options, "", func(v *AvailableDevice, _ types.EndpointBinding) error { return v.Validate() })
	if e != nil {
		return pagination.Collection[AvailableDevice]{}, e
	}
	return iterator.Collect(ctx)
}
