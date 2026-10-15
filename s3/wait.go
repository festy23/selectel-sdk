package s3

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"strings"
)

func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	kind := "bucket"
	if kindMatches(ref.Kind, "object_key") {
		kind = "object"
		if e := c.validObject(ref); e != nil {
			return nil, e
		}
	} else if kindMatches(ref.Kind, "bucket_name") {
		if e := c.valid(ref, "bucket_name"); e != nil {
			return nil, e
		}
	} else {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	goals := map[string]operations.Goal{kind + ".present": {Success: map[string]bool{"present": true}}, kind + ".absent": {AbsentSuccess: true, Pending: map[string]bool{"present": true}}}
	if receipt != nil {
		cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		allowed := map[string]operations.Goal{}
		for _, g := range cap.WaitGoals {
			if v, ok := goals[g]; ok {
				allowed[g] = v
			}
		}
		goals = allowed
	}
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		var meta types.Metadata
		var e error
		if kindMatches(ref.Kind, "bucket_name") {
			v, err := c.Buckets.Head(ctx, ref, types.CallOptions{})
			meta, e = v.Metadata, err
		} else {
			v, err := c.Objects.Head(ctx, ref, HeadObjectOptions{}, types.CallOptions{})
			meta, e = v.Metadata, err
		}
		if e != nil {
			return operations.Observation{}, e
		}
		return operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", Metadata: meta, ObservedAt: meta.ObservedAt}, nil
	}, goals)
}
func (c *Client) Resume(r types.Receipt) (*operations.Handle, error) {
	if r.SchemaVersion != 1 || r.CallID == "" || r.Scope != c.scope || r.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	cap, e := c.runtime.Catalog().Describe(r.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.Service != "s3" || cap.APIGeneration != generation || cap.ContractRevision != r.ContractRevision || cap.Effect != "mutation" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	switch r.Outcome {
	case types.Accepted, types.Rejected, types.NotSent, types.Unknown:
	default:
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(r.ResourceRefs) == 0 {
		if r.Outcome == types.Unknown {
			return nil, types.NewError(types.RecoveryUnavailable)
		}
		return operations.BindReceipt(c.runtime, &r)
	}
	if len(r.ResourceRefs) != 1 || r.ProviderOperationRef != nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	target := r.ResourceRefs[0]
	kind := "object_key"
	if strings.HasPrefix(r.OperationID, "s3."+generation+".buckets.") || r.OperationID == "s3."+generation+".objects.delete_many" {
		kind = "bucket_name"
	}
	if !kindMatches(target.Kind, kind) {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if kind == "object_key" {
		if e := c.validObject(target); e != nil {
			return nil, e
		}
	} else if e := c.valid(target, kind); e != nil {
		return nil, e
	}
	if ep := r.Endpoint; ep != nil {
		if ep.Scope != r.Scope || ep.Audience != "selectel" || ep.APIGeneration != generation || ep.ID != target.EndpointID || ep.URL != target.EndpointURL {
			return nil, types.NewError(types.ScopeMismatch)
		}
	} else if r.Outcome == types.Unknown {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if r.Outcome == types.Unknown && (r.Recovery != "resource" || r.OperationID == "s3."+generation+".objects.delete_many") {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	return c.Handle(target, &r)
}
