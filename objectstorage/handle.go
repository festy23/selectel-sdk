package objectstorage

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"strings"
)

func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if ref.Kind != "container_name" && ref.Kind != "log_export" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if e := c.validateRef(ref, ref.Kind); e != nil {
		return nil, e
	}
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		var meta types.Metadata
		var e error
		if ref.Kind == "container_name" {
			r, err := c.Containers.Options(ctx, ref, types.CallOptions{})
			meta, e = r.Metadata, err
		} else {
			r, err := c.LogExports.Get(ctx, ref, types.CallOptions{})
			meta, e = r.Metadata, err
		}
		if e != nil {
			return operations.Observation{}, e
		}
		return operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", ObservedAt: meta.ObservedAt, Metadata: meta}, nil
	}, map[string]operations.Goal{})
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != c.scope || receipt.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.Service != "object_storage" || cap.APIGeneration != "v2" || cap.ContractRevision != receipt.ContractRevision || cap.Effect != "mutation" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if receipt.Outcome != types.Accepted && receipt.Outcome != types.Rejected && receipt.Outcome != types.NotSent && receipt.Outcome != types.Unknown {
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(receipt.ResourceRefs) == 0 {
		if receipt.Outcome == types.Unknown {
			return nil, types.NewError(types.RecoveryUnavailable)
		}
		return operations.BindReceipt(c.runtime, &receipt)
	}
	if len(receipt.ResourceRefs) != 1 || receipt.ProviderOperationRef != nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	ref := receipt.ResourceRefs[0]
	kind := "container_name"
	if strings.HasPrefix(receipt.OperationID, "object_storage.v2.log_exports.") {
		kind = "log_export"
	}
	if e := c.validateRef(ref, kind); e != nil {
		return nil, e
	}
	if receipt.Endpoint != nil {
		ep := receipt.Endpoint
		if ep.ID != ref.EndpointID || ep.URL != ref.EndpointURL || ep.Scope != receipt.Scope || ep.APIGeneration != "v2" || ep.Audience != "selectel" {
			return nil, types.NewError(types.ScopeMismatch)
		}
	} else if receipt.Outcome == types.Unknown {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if receipt.Outcome == types.Unknown && receipt.Recovery != "resource" {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	return c.Handle(ref, &receipt)
}
