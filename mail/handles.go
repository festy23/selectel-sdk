package mail

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refError(ref); e != nil {
		return nil, e
	}
	goals := map[string]operations.Goal{"resource_active": {Success: map[string]bool{"active": true}, Failure: map[string]bool{"blocked": true, "error": true, "deleted": true}}, "resource_absent": {AbsentSuccess: true, Pending: map[string]bool{"active": true, "blocked": true, "error": true, "deleted": true}}}
	if receipt != nil {
		capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Endpoint == nil || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.APIGeneration != "v1" || receipt.Endpoint.Audience != "selectel" || receipt.ProviderOperationRef != nil || receipt.Scope != c.scope || len(receipt.ResourceRefs) != 1 || receipt.ResourceRefs[0] != ref || capability.Service != "email" || capability.APIGeneration != "v1" || capability.Effect != "mutation" || capability.ContractRevision != receipt.ContractRevision {
			return nil, types.NewError(types.InvalidArgument)
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		allowed := map[string]operations.Goal{}
		for _, id := range capability.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		out, e := c.GetResource(ctx, ref, types.CallOptions{})
		state := ""
		if out.Data.Resource != nil && out.Data.Resource.Status != nil {
			state = string(*out.Data.Resource.Status)
		}
		classification := "unknown"
		if state == "active" {
			classification = "succeeded"
		}
		if state == "blocked" || state == "error" || state == "deleted" {
			classification = "failed"
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: classification, Metadata: out.Metadata, ObservedAt: out.Metadata.ObservedAt}, e
	}, goals)
}
