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
	if receipt.OperationID == domainTransferOperation {
		selected, e := domainTransferTarget(receipt)
		if e != nil {
			return nil, e
		}
		return c.ClusterDomainHandle(*selected, &receipt)
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
	case "cluster_volume":
		return c.ClusterVolumeHandle(*selected, &receipt)
	case "cluster_domain":
		return c.ClusterDomainHandle(*selected, &receipt)
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
}

func validateReceiptReferenceSet(receipt types.Receipt, selected types.ResourceRef) error {
	if receipt.OperationID == domainTransferOperation {
		target, e := domainTransferTarget(receipt)
		if e != nil {
			return e
		}
		if !reflect.DeepEqual(*target, selected) {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}
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

const domainTransferOperation = "cloud_1c.v2.clusters_domains_transfer.create"

// A transfer receipt has an explicit target anchor, original child and returned
// target child. Never construct the target child from the request alone.
func domainTransferTarget(receipt types.Receipt) (*types.ResourceRef, error) {
	if receipt.Endpoint == nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	var target *types.ResourceRef
	var children []types.ResourceRef
	for _, ref := range receipt.ResourceRefs {
		if e := ref.Validate(); e != nil {
			return nil, e
		}
		if ref.Scope != receipt.Scope || ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.EndpointID != receipt.Endpoint.ID || ref.EndpointURL != receipt.Endpoint.URL {
			return nil, types.NewError(types.ScopeMismatch)
		}
		switch ref.Kind {
		case "cluster":
			if target != nil || ref.Parent != nil {
				return nil, types.NewError(types.ScopeMismatch)
			}
			target = cloneResourceRef(&ref)
		case "cluster_domain":
			if ref.Parent == nil || ref.Parent.Kind != "cluster" || ref.Parent.Parent != nil || ref.Parent.Scope != ref.Scope || ref.Parent.Service != ref.Service || ref.Parent.APIGeneration != ref.APIGeneration || ref.Parent.EndpointID != ref.EndpointID || ref.Parent.EndpointURL != ref.EndpointURL {
				return nil, types.NewError(types.ScopeMismatch)
			}
			children = append(children, ref)
		default:
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	if target == nil || len(children) != 2 {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if children[0].ID != children[1].ID || reflect.DeepEqual(children[0], children[1]) {
		return nil, types.NewError(types.ScopeMismatch)
	}
	var selected *types.ResourceRef
	for _, child := range children {
		if reflect.DeepEqual(*child.Parent, *target) {
			if selected != nil {
				return nil, types.NewError(types.ScopeMismatch)
			}
			selected = cloneResourceRef(&child)
		}
	}
	if selected == nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	return selected, nil
}
