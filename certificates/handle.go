package certificates

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
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
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		result, e := c.Get(ctx, ref, types.CallOptions{})
		if e != nil {
			// Only an error after resolving the exact resource endpoint can
			// establish resource absence. A404 at identity is not that proof.
			if types.ErrorCode(e) == types.NotFound && result.Metadata.EndpointID == "" {
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
		return operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, e
	}, nil)
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
	if cap.Service != "certificates" || cap.APIGeneration != "user_v1" || cap.Effect != "mutation" {
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
	if ep.Scope != c.scope || ep.APIGeneration != "user_v1" || ep.Audience != "selectel" || ep.ID != ref.EndpointID || ep.URL != ref.EndpointURL {
		return nil, types.NewError(types.ScopeMismatch)
	}
	return c.bindHandle(ref, &receipt)
}
