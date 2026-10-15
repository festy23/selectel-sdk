package letsencrypt

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
)

func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.valid(ref); e != nil {
		return nil, e
	}
	if receipt != nil {
		handle, e := c.Resume(*receipt)
		if e != nil {
			return nil, e
		}
		if !reflect.DeepEqual(handle.Resource(), ref) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		return handle, nil
	}
	return c.bindHandle(ref, nil)
}
func (c *Client) bindHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	goals := map[string]operations.Goal{
		"active":  {Success: map[string]bool{"active": true}, Failure: map[string]bool{"error": true, "invalid": true, "deleted": true}, Pending: map[string]bool{"creating": true, "validating": true, "issuing": true, "renewing": true}},
		"deleted": {Success: map[string]bool{"deleted": true, "absent": true}, Pending: map[string]bool{"creating": true, "validating": true, "issuing": true, "active": true, "renewing": true, "error": true, "invalid": true}},
	}
	if receipt != nil {
		if receipt.OperationID == "certificates.le_v1.certificates.delete" {
			delete(goals, "active")
		} else {
			delete(goals, "deleted")
		}
	}
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		op, e := c.operation("certificates.le_v1.certificates.list")
		if e != nil {
			return operations.Observation{}, e
		}
		if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
			return operations.Observation{}, types.NewError(types.ScopeMismatch)
		}
		iterator, e := c.List(pagination.Options{})
		if e != nil {
			return operations.Observation{}, e
		}
		result, e := iterator.Collect(ctx)
		if e != nil {
			// A 404 from auth or the collection endpoint is not proof that an exact resource is absent.
			if types.ErrorCode(e) == types.NotFound {
				if detail, ok := e.(*types.SDKError); ok {
					copy := *detail
					copy.Code = types.EndpointUnavailable
					e = &copy
				} else {
					e = types.NewError(types.EndpointUnavailable)
				}
			}
			return operations.Observation{}, e
		}
		for _, item := range result.Items {
			if item.Ref.ID != ref.ID {
				continue
			}
			state := ""
			if item.Status != nil {
				state = *item.Status
			}
			classification := "unknown"
			switch state {
			case "active":
				classification = "succeeded"
			case "creating", "validating", "issuing", "renewing":
				classification = "pending"
			case "error", "invalid":
				classification = "failed"
			case "deleted":
				classification = "absent"
			}
			return operations.Observation{Ref: ref, ProviderState: state, Classification: classification, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
		}
		if !result.Complete {
			return operations.Observation{}, types.NewError(types.Incomplete)
		}
		return operations.Observation{Ref: ref, ProviderState: "absent", Classification: "absent", ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil

	}, goals)
}
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
	if cap.ContractRevision != receipt.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if cap.Service != "certificates" || cap.APIGeneration != "le_v1" || cap.Effect != "mutation" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if _, e := c.operation(receipt.OperationID); e != nil {
		return nil, e
	}
	if receipt.Outcome != types.Accepted && receipt.Outcome != types.NotSent && receipt.Outcome != types.Rejected && receipt.Outcome != types.Unknown {
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(receipt.ResourceRefs) == 0 && receipt.ProviderOperationRef == nil && receipt.Outcome != types.Unknown {
		return operations.BindReceipt(c.runtime, &receipt)
	}
	if len(receipt.ResourceRefs) != 1 || receipt.Endpoint == nil || receipt.ProviderOperationRef != nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	ref := receipt.ResourceRefs[0]
	if e := c.valid(ref); e != nil {
		return nil, e
	}
	ep := receipt.Endpoint
	if ep.ID == "" || types.ValidateEndpoint(ep.URL) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if ep.Scope != c.scope || ep.APIGeneration != "le_v1" || ep.Audience != "selectel" || ep.ID != ref.EndpointID || ep.URL != ref.EndpointURL {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return c.bindHandle(ref, &receipt)
}
