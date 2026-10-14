package opensearch

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"strings"
)

// Resource observes an exact OpenSearch reference without replaying a mutation.
func (c *Datastores) Resource(ref types.ResourceRef) (*operations.Handle, error) {
	return c.handle(ref, nil)
}
func (c *Datastores) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || !strings.HasPrefix(receipt.OperationID, "databases.v2.opensearch.") {
		return nil, types.NewError(types.ScopeMismatch)
	}
	capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if capability.Effect != "mutation" || receipt.Scope != c.scope || receipt.Endpoint == nil || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if receipt.ContractRevision != capability.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if len(receipt.ResourceRefs) != 1 {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	ref := receipt.ResourceRefs[0]
	if e := c.refValidation(ref); e != nil {
		return nil, e
	}
	if receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return c.handle(ref, &receipt)
}
func (c *Datastores) handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refValidation(ref); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(ref)
	var owned types.ResourceRef
	_ = json.Unmarshal(raw, &owned)
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := c.Get(ctx, owned)
		if e != nil {
			return operations.Observation{}, e
		}
		if result.Data.Status == nil {
			return operations.Observation{}, types.NewError(types.ProtocolError)
		}
		status := *result.Data.Status
		classification := "unknown"
		switch status {
		case "ACTIVE":
			classification = "success"
		case "ERROR":
			classification = "failure"
		case "CREATING", "UPDATING", "RESIZING", "DELETING", "DELETED", "LOCKING", "LOCKED", "UNLOCKING", "REBUILDING", "MAINTENANCE":
			classification = "pending"
		}
		return operations.Observation{Ref: owned, ProviderState: status, Classification: classification, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	pending := map[string]bool{}
	for _, status := range []string{"CREATING", "UPDATING", "RESIZING", "DELETING", "LOCKING", "LOCKED", "UNLOCKING", "REBUILDING", "MAINTENANCE"} {
		pending[status] = true
	}
	deletedPending := map[string]bool{"ACTIVE": true, "DELETED": true}
	for status := range pending {
		deletedPending[status] = true
	}
	goals := map[string]operations.Goal{"opensearch.active": {Success: map[string]bool{"ACTIVE": true}, Failure: map[string]bool{"ERROR": true, "DELETED": true}, Pending: pending}, "opensearch.deleted": {Failure: map[string]bool{"ERROR": true}, Pending: deletedPending, AbsentSuccess: true}}
	if receipt != nil {
		capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		allowed := map[string]operations.Goal{}
		for _, goal := range capability.WaitGoals {
			if value, ok := goals[goal]; ok {
				allowed[goal] = value
			}
		}
		goals = allowed
	}
	return operations.Bind(c.runtime, owned, receipt, read, goals)
}
