package onec

import (
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
)

// Resume selects one exact compiled resource, permitting only its explicitly
// recorded ancestors as additional refs. A parent anchor never substitutes for
// a missing created child ID.
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" || receipt.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if receipt.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.ContractRevision != receipt.ContractRevision || cap.Service != "cloud_1c" || cap.APIGeneration != "v2" || cap.Effect != "mutation" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	switch receipt.Outcome {
	case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
	default:
		return nil, types.NewError(types.InvalidArgument)
	}
	if receipt.Endpoint == nil || len(receipt.ResourceRefs) == 0 {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if receipt.Endpoint.Scope != c.scope || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Audience != "selectel" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if receipt.ProviderOperationRef != nil {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	kind := receiptResourceKind(receipt.OperationID)
	if kind == "" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	var selected *types.ResourceRef
	for _, ref := range receipt.ResourceRefs {
		if e := ref.Validate(); e != nil {
			return nil, e
		}
		if ref.Scope != c.scope || ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.EndpointID != receipt.Endpoint.ID || ref.EndpointURL != receipt.Endpoint.URL {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if ref.Kind == kind {
			if selected != nil {
				return nil, types.NewError(types.RecoveryUnavailable)
			}
			selected = cloneResourceRef(&ref)
		}
	}
	if selected == nil {
		if kind == "cluster" || kind == "datastore" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	for _, ref := range receipt.ResourceRefs {
		if reflect.DeepEqual(ref, *selected) {
			continue
		}
		found := false
		for parent := selected.Parent; parent != nil; parent = parent.Parent {
			if reflect.DeepEqual(ref, *parent) {
				found = true
				break
			}
		}
		if !found {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	switch kind {
	case "cluster":
		return c.ClusterHandle(*selected, &receipt)
	case "datastore":
		return c.DatastoreHandle(*selected, &receipt)
	case "cluster_proxy":
		return c.ClusterProxyHandle(*selected, &receipt)
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
}

func validateReceiptReferenceSet(receipt types.Receipt, selected types.ResourceRef) error {
	exact := 0
	for _, ref := range receipt.ResourceRefs {
		if e := ref.Validate(); e != nil {
			return e
		}
		if ref.Scope != selected.Scope || ref.Service != selected.Service || ref.APIGeneration != selected.APIGeneration || ref.EndpointID != selected.EndpointID || ref.EndpointURL != selected.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		if reflect.DeepEqual(ref, selected) {
			exact++
			continue
		}
		found := false
		for parent := selected.Parent; parent != nil; parent = parent.Parent {
			if reflect.DeepEqual(ref, *parent) {
				found = true
				break
			}
		}
		if !found {
			return types.NewError(types.ScopeMismatch)
		}
	}
	if exact != 1 {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
