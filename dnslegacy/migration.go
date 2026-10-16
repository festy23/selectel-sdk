package dnslegacy

import (
	"context"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// MigrateToV2 is the source's explicit copy action. It neither switches the
// client generation nor adopts destination resources or proves v2 readiness.
func (c *Client) MigrateToV2(ctx context.Context, spec MigrationSpec, o types.CallOptions) (types.Result[struct{}], error) {
	op, bindErr := c.runtime.BindDocumented(core.Operation{ID: "dns.v1.version.replace", Service: "dns", APIGeneration: "v1", Method: "PUT", Effect: "mutation", Auth: types.StaticToken, Success: []int{204}, NoContentSuccess: []int{204}, Rejected: []int{400, 401, 403, 404, 409, 422}}, c.scope)
	body, bodyErr := wireBody(spec)
	validate := func() error {
		if bindErr != nil {
			return bindErr
		}
		if spec.Validate() != nil || spec.ProjectID == nil || spec.ProjectID.Empty() {
			return types.NewError(types.InvalidArgument)
		}
		for _, id := range spec.Domains {
			if !decimalID.MatchString(id.String()) {
				return types.NewError(types.InvalidArgument)
			}
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/version/v2", Body: body, Options: o, Validate: validate}, func(_ []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
