package dns

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"strings"
)

// Handle observes existence in the saved authorization view, not DNS propagation.
func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	s := c.Zones.service
	if ref.Kind != "zone" && ref.Kind != "rrset" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if e := s.valid(ref, ref.Kind); e != nil {
		return nil, e
	}
	goals := map[string]operations.Goal{ref.Kind + ".present": {Success: map[string]bool{"present": true}}, ref.Kind + ".absent": {AbsentSuccess: true, Pending: map[string]bool{"present": true}}}
	if receipt != nil {
		cap, e := s.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		allowed := map[string]operations.Goal{}
		for _, g := range cap.WaitGoals {
			if value, ok := goals[g]; ok {
				allowed[g] = value
			}
		}
		goals = allowed
	}
	return operations.Bind(s.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		var meta types.Metadata
		var e error
		if ref.Kind == "zone" {
			v, err := c.Zones.Get(ctx, ref, types.CallOptions{})
			e = err
			meta = v.Metadata
		} else {
			v, err := c.RRSets.Get(ctx, ref, types.CallOptions{})
			e = err
			meta = v.Metadata
		}
		if e != nil {
			return operations.Observation{}, e
		}
		return operations.Observation{Ref: ref, ProviderState: "present", Classification: "succeeded", ObservedAt: meta.ObservedAt, Metadata: meta}, nil
	}, goals)
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	s := c.Zones.service
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != s.scope || receipt.Scope.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	cap, e := s.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.Service != "dns" || cap.APIGeneration != "v2" || cap.Effect != "mutation" || cap.ContractRevision != receipt.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	switch receipt.Outcome {
	case types.Accepted, types.Rejected, types.NotSent, types.Unknown:
	default:
		return nil, types.NewError(types.InvalidArgument)
	}
	if len(receipt.ResourceRefs) == 0 {
		if receipt.Outcome != types.Unknown {
			return operations.BindReceipt(s.runtime, &receipt)
		}
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if receipt.ProviderOperationRef != nil {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if receipt.Endpoint == nil && receipt.Outcome == types.Unknown {
		return nil, types.NewError(types.RecoveryUnavailable)
	}
	if ep := receipt.Endpoint; ep != nil {
		if ep.Scope != s.scope || ep.Audience != "selectel" || ep.APIGeneration != "v2" || ep.ID == "" || types.ValidateEndpoint(ep.URL) != nil {
			return nil, types.NewError(types.ScopeMismatch)
		}
	}
	kind := "zone"
	if strings.HasPrefix(receipt.OperationID, "dns.v2.rrsets.") {
		kind = "rrset"
	}
	var target *types.ResourceRef
	for i := range receipt.ResourceRefs {
		r := receipt.ResourceRefs[i]
		if e := s.valid(r, r.Kind); e != nil {
			return nil, e
		}
		if r.Kind != "zone" && r.Kind != "rrset" {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if ep := receipt.Endpoint; ep != nil && (r.EndpointID != ep.ID || r.EndpointURL != ep.URL) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if r.Kind == kind {
			if target != nil {
				return nil, types.NewError(types.RecoveryUnavailable)
			}
			copy := r
			target = &copy
		}
	}
	if target == nil {
		if receipt.Outcome == types.Unknown {
			return nil, types.NewError(types.RecoveryUnavailable)
		}
		return operations.Bind(s.runtime, receipt.ResourceRefs[0], &receipt, func(context.Context) (operations.Observation, error) {
			return operations.Observation{}, types.NewError(types.RecoveryUnavailable)
		}, nil)
	}
	return c.Handle(*target, &receipt)
}
