package registrydata

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
	"time"
)

// Handle observes an explicit address; presence is neither ownership nor proof
// that an unknown mutation created or replaced its content. HEAD is not a digest
// verification of downloaded bytes. No asynchronous wait goals are inferred.
func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if receipt == nil {
		return c.bindHandle(ref, nil)
	}
	handle, e := c.Resume(*receipt)
	if e != nil {
		return nil, e
	}
	a, _ := json.Marshal(ref)
	b, _ := json.Marshal(handle.Resource())
	if string(a) != string(b) {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return handle, nil
}
func (c *Client) bindHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	switch ref.Kind {
	case "manifest_tag", "manifest_digest":
		if e := c.Manifests.valid(ref); e != nil {
			return nil, e
		}
	case "blob":
		if e := c.Blobs.valid(ref); e != nil {
			return nil, e
		}
	case "upload", "repository":
		if e := c.valid(ref, ref.Kind); e != nil {
			return nil, e
		}
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	// Prevent caller mutation of nested parent pointers after binding.
	saved, _ := json.Marshal(ref)
	_ = json.Unmarshal(saved, &ref)
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		out := operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", ObservedAt: time.Now().UTC()}
		switch ref.Kind {
		case "manifest_tag", "manifest_digest":
			result, e := c.Manifests.Head(ctx, ref, ManifestReadOptions{}, types.CallOptions{})
			out.Metadata = result.Metadata
			out.ObservedAt = result.Metadata.ObservedAt
			return out, e
		case "blob":
			result, e := c.Blobs.Head(ctx, ref, types.CallOptions{})
			out.Metadata = result.Metadata
			out.ObservedAt = result.Metadata.ObservedAt
			return out, e
		default:
			return operations.Observation{}, types.NewError(types.RecoveryUnavailable)
		}
	}, nil)
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope.Validate() != nil || receipt.ContractRevision == "" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if receipt.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.ContractRevision != receipt.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if cap.Service != "registry_data" || cap.APIGeneration != "oci_v1" || cap.Effect != "mutation" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if receipt.Outcome != types.Accepted && receipt.Outcome != types.NotSent && receipt.Outcome != types.Rejected && receipt.Outcome != types.Unknown {
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(receipt.ResourceRefs) == 0 && receipt.ProviderOperationRef == nil && receipt.Outcome != types.Unknown {
		return operations.BindReceipt(c.runtime, &receipt)
	}
	if receipt.ProviderOperationRef != nil || receipt.Endpoint == nil || len(receipt.ResourceRefs) == 0 || len(receipt.ResourceRefs) > 4 || (len(receipt.ResourceRefs) > 2 && receipt.OperationID != "registry_data.oci_v1.uploads.start") {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	ep := receipt.Endpoint
	if ep.ID == "" || types.ValidateEndpoint(ep.URL) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if ep.Scope != c.scope || ep.APIGeneration != "oci_v1" || ep.Audience != "selectel" {
		return nil, types.NewError(types.ScopeMismatch)
	}
	first := receipt.ResourceRefs[0]
	expected := ""
	switch receipt.OperationID {
	case "registry_data.oci_v1.uploads.start":
		expected = "repository"
	case "registry_data.oci_v1.uploads.append", "registry_data.oci_v1.uploads.finalize", "registry_data.oci_v1.uploads.cancel_native":
		expected = "upload"
	case "registry_data.oci_v1.blobs.delete":
		expected = "blob"
	case "registry_data.oci_v1.manifests.replace", "registry_data.oci_v1.manifests.delete":
		expected = first.Kind
		if expected != "manifest_tag" && expected != "manifest_digest" {
			return nil, types.NewError(types.InvalidArgument)
		}
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if first.Kind != expected {
		return nil, types.NewError(types.InvalidArgument)
	}
	for _, ref := range receipt.ResourceRefs {
		if e := c.valid(ref, ref.Kind); e != nil {
			return nil, e
		}
		if ref.EndpointID != ep.ID || ref.EndpointURL != ep.URL {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	if receipt.OperationID == "registry_data.oci_v1.uploads.start" {
		primary, e := startReceiptPrimary(receipt.ResourceRefs)
		if e != nil {
			return nil, e
		}
		return c.bindHandle(primary, &receipt)
	}
	primary := first
	if len(receipt.ResourceRefs) == 2 {
		second := receipt.ResourceRefs[1]
		if receipt.OperationID != "registry_data.oci_v1.uploads.finalize" || second.Kind != "blob" {
			return nil, types.NewError(types.InvalidArgument)
		}
		if first.Parent == nil || second.Parent == nil || !reflect.DeepEqual(*second.Parent, *first.Parent) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if _, e := ParseDigest(second.ID); e != nil {
			return nil, e
		}
		primary = second
	}

	return c.bindHandle(primary, &receipt)
}

// startReceiptPrimary validates the explicit variants of the same remote POST:
// repository[,requested blob][,source repository][,accepted session/canonical blob].
func startReceiptPrimary(refs []types.ResourceRef) (types.ResourceRef, error) {
	first := refs[0]
	if len(refs) == 1 {
		return first, nil
	}
	second := refs[1]
	if second.Parent == nil || !reflect.DeepEqual(*second.Parent, first) {
		return types.ResourceRef{}, types.NewError(types.ScopeMismatch)
	}
	if len(refs) == 2 && second.Kind == "upload" {
		return first, nil
	} // original Start shape
	if second.Kind != "blob" {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	if _, e := ParseDigest(second.ID); e != nil {
		return types.ResourceRef{}, e
	}
	primary := second
	remaining := refs[2:]
	if len(remaining) > 0 && remaining[0].Kind == "repository" {
		remaining = remaining[1:]
	}
	if len(remaining) > 1 {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	if len(remaining) == 1 {
		accepted := remaining[0]
		if accepted.Kind != "blob" && accepted.Kind != "upload" {
			return types.ResourceRef{}, types.NewError(types.InvalidArgument)
		}
		if accepted.Parent == nil || !reflect.DeepEqual(*accepted.Parent, first) {
			return types.ResourceRef{}, types.NewError(types.ScopeMismatch)
		}
		if accepted.Kind == "blob" {
			if _, e := ParseDigest(accepted.ID); e != nil {
				return types.ResourceRef{}, e
			}
		}
		primary = accepted
	}
	return primary, nil
}
