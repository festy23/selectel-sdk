package registry

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

// Handle observes token metadata, not possession/validity of a secret or success
// of credential rotation. Unknown outcomes always remain indeterminate.
func (c *V2Tokens) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := tokenRefValid(ref, c.registries.scope); e != nil {
		return nil, e
	}
	goals := map[string]operations.Goal{"exists": {Success: map[string]bool{"active": true, "expired": true, "deleted": true}}, "absent": {AbsentSuccess: true, Pending: map[string]bool{"active": true, "expired": true, "deleted": true}}}
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" || receipt.Scope.Validate() != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receipt.Scope != ref.Scope {
			return nil, types.NewError(types.ScopeMismatch)
		}
		cap, e := c.registries.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if cap.ContractRevision != receipt.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		definition, ok := tokenOperations[receipt.OperationID]
		if !ok || definition.APIGeneration != "v2" || definition.Effect != "mutation" || cap.Effect != "mutation" || cap.APIGeneration != "v2" || cap.Service != "registry" {
			return nil, types.NewError(types.InvalidArgument)
		}
		switch receipt.Outcome {
		case types.Accepted, types.NotSent, types.Rejected, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		if len(receipt.ResourceRefs) != 1 || receipt.Endpoint == nil || receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.RecoveryUnavailable)
		}
		if receipt.ResourceRefs[0] != ref || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Audience != "selectel" {
			return nil, types.NewError(types.ScopeMismatch)
		}
		goals = map[string]operations.Goal{}
		if receipt.OperationID == "registry.v2.tokens.delete" {
			goals["absent"] = operations.Goal{AbsentSuccess: true, Pending: map[string]bool{"active": true, "expired": true, "deleted": true}}
		}
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.Get(ctx, ref, types.CallOptions{})
		state := ""
		if v.Data.Status != nil {
			state = *v.Data.Status
		}
		classification := "unknown"
		if state == "active" || state == "expired" || state == "deleted" {
			classification = "succeeded"
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: classification, Metadata: v.Metadata, ObservedAt: v.Metadata.ObservedAt}, e
	}
	return operations.Bind(c.registries.runtime, ref, receipt, read, goals)
}
