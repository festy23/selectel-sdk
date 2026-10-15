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
	if receipt.ProviderOperationRef != nil || receipt.Endpoint == nil || len(receipt.ResourceRefs) == 0 || len(receipt.ResourceRefs) > 3 || (len(receipt.ResourceRefs) == 3 && receipt.OperationID != "registry_data.oci_v1.uploads.start") {
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
	if len(receipt.ResourceRefs) >= 2 {
		second := receipt.ResourceRefs[1]
		parent := first
		if receipt.OperationID == "registry_data.oci_v1.uploads.start" {
			if second.Kind != "upload" && second.Kind != "blob" {
				return nil, types.NewError(types.InvalidArgument)
			}
			if second.Kind == "blob" {
				if _, e := ParseDigest(second.ID); e != nil {
					return nil, e
				}
			}
			if len(receipt.ResourceRefs) == 3 {
				third := receipt.ResourceRefs[2]
				if second.Kind != "blob" || third.Kind != "upload" || third.Parent == nil || !reflect.DeepEqual(*third.Parent, first) {
					return nil, types.NewError(types.InvalidArgument)
				}
			}
		} else if receipt.OperationID == "registry_data.oci_v1.uploads.finalize" {
			if second.Kind != "blob" {
				return nil, types.NewError(types.InvalidArgument)
			}
			parent = *first.Parent
			if _, e := ParseDigest(second.ID); e != nil {
				return nil, e
			}
		} else {
			return nil, types.NewError(types.InvalidArgument)
		}
		a, _ := json.Marshal(second.Parent)
		b, _ := json.Marshal(parent)
		if string(a) != string(b) {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	primary := first
	if receipt.OperationID == "registry_data.oci_v1.uploads.start" && (len(receipt.ResourceRefs) == 3 || (len(receipt.ResourceRefs) == 2 && receipt.ResourceRefs[1].Kind == "blob")) {
		primary = receipt.ResourceRefs[len(receipt.ResourceRefs)-1]
	}
	if receipt.OperationID == "registry_data.oci_v1.uploads.finalize" && len(receipt.ResourceRefs) == 2 {
		primary = receipt.ResourceRefs[1]
	}
	return c.bindHandle(primary, &receipt)
}
