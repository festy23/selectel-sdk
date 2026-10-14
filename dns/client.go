// Package dns manages current public DNS v2 zones and resource record sets.
package dns

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type service struct {
	runtime *core.Runtime
	scope   types.Scope
}
type Client struct {
	Zones  *Zones
	RRSets *RRSets
}
type Zones struct{ service }

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{Zones: &Zones{service{r, s}}, RRSets: &RRSets{service{r, s}}}
}
func op(resource, action, method string, success ...int) core.Operation {
	effect := "mutation"
	if method == "GET" {
		effect = "read"
	}
	operation := core.Operation{ID: "dns.v2." + resource + "." + action, Service: "dns", APIGeneration: "v2", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, EndpointID: "selectel-dns-v2", FixedEndpoint: "https://api.selectel.ru/domains/v2", Success: success, Rejected: []int{400, 401, 403, 404, 409, 422}, ErrorCodes: map[int]types.Code{400: types.InvalidArgument, 422: types.InvalidArgument}}
	for _, n := range success {
		if n == 204 {
			operation.NoContentSuccess = []int{204}
		}
	}
	return operation
}
func (c *Zones) Create(ctx context.Context, spec ZoneCreate, o types.CallOptions) (types.Result[Zone], error) {
	body, e := json.Marshal(spec)
	return core.DoJSON(c.runtime, ctx, op("zones", "create", "POST", 200), core.Request{Scope: c.scope, Path: "/zones", Body: body, Options: o, Validate: func() error {
		if e != nil {
			return types.NewError(types.InvalidArgument)
		}
		if !fqdn(spec.Name) {
			return types.NewError(types.InvalidArgument)
		}
		return c.scopeValid()
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Zone], error) {
		return decodeZone(raw, ep, "")
	})
}
