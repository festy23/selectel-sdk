package registry

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
)

// Handle observes a pinned v1 registry management address. Presence is not
// garbage-collection completion, cleanup success, or alias-to-digest proof.
func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	scope := c.V1.Registries.scope
	if ref.APIGeneration != "v1" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	switch ref.Kind {
	case "registry", "repository", "manifest", "image_reference":
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if e := managementValid(ref, scope, "v1", ref.Kind); e != nil {
		return nil, e
	}
	goals := map[string]operations.Goal{"exists": {Success: map[string]bool{"present": true}}, "absent": {AbsentSuccess: true, Pending: map[string]bool{"present": true}}}
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" || receipt.Scope.Validate() != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receipt.Scope != scope {
			return nil, types.NewError(types.ScopeMismatch)
		}
		cap, e := c.V1.Registries.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if cap.ContractRevision != receipt.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if cap.Effect != "mutation" || cap.APIGeneration != "v1" || cap.Service != "registry" {
			return nil, types.NewError(types.InvalidArgument)
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		if len(receipt.ResourceRefs) != 1 || receipt.Endpoint == nil || receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.RecoveryUnavailable)
		}
		if !reflect.DeepEqual(receipt.ResourceRefs[0], ref) || receipt.Endpoint.Scope != scope || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.APIGeneration != "v1" || receipt.Endpoint.Audience != "selectel" {
			return nil, types.NewError(types.ScopeMismatch)
		}
		expected, ok := managementRefKinds[receipt.OperationID]
		if receipt.OperationID == "registry.v1.registries.create" || receipt.OperationID == "registry.v1.registries.delete" {
			expected = "registry"
			ok = true
		}
		if !ok || ref.Kind != expected {
			return nil, types.NewError(types.InvalidArgument)
		}
		goals = map[string]operations.Goal{}
		if receipt.OperationID == "registry.v1.registries.delete" || receipt.OperationID == "registry.v1.registries_repositories.delete" || receipt.OperationID == "registry.v1.registries_repositories.delete_9116f718" {
			goals["absent"] = operations.Goal{AbsentSuccess: true, Pending: map[string]bool{"present": true}}
		}
	}
	// operations.Bind owns a deep snapshot; read closure must own the same snapshot.
	ref = snapshotRegistryRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		var meta types.Metadata
		var e error
		switch ref.Kind {
		case "registry":
			v, err := c.V1.Registries.Get(ctx, ref, types.CallOptions{})
			meta, e = v.Metadata, err
		case "repository":
			v, err := c.V1.Repositories().Get(ctx, ref, types.CallOptions{})
			meta, e = v.Metadata, err
		default:
			v, err := c.V1.Images().Layers(ctx, ref, types.CallOptions{})
			meta, e = v.Metadata, err
		}
		return operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", ObservedAt: meta.ObservedAt, Metadata: meta}, e
	}
	return operations.Bind(c.V1.Registries.runtime, ref, receipt, read, goals)
}
