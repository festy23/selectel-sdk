package mobilefarm

import (
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type authPolicy uint8

const (
	projectOnly authPolicy = iota
	accountOnly
	accountOrProject
)

// Policies are compiled from each native operation's source security list. View
// scope selects one allowed IAM scheme; there is never a fallback or scope change.
func (c *Client) binding(id, method, effect string, policy authPolicy, success, rejected, noContent []int) (core.Operation, error) {
	mode := types.IAMAccount
	if policy == projectOnly || policy == accountOrProject && c.scope.ProjectID != "" {
		mode = types.IAMProject
	}
	op := core.Operation{ID: id, Service: "mobile_farm", APIGeneration: "v3", Method: method, Effect: effect, Auth: mode, RequireProject: policy == projectOnly, Success: success, Rejected: rejected, NoContentSuccess: noContent}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return op, e
	}
	if policy == accountOnly && c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
		return op, types.NewError(types.ScopeMismatch)
	}
	return op, nil
}
