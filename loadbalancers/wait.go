package loadbalancers

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

func segment(id string) string {
	if id == "." || id == ".." {
		return strings.ReplaceAll(id, ".", "%2E")
	}
	return url.PathEscape(id)
}
func kindFor(resource string) string {
	return map[string]string{"loadbalancers": "loadbalancer", "listeners": "listener", "pools": "pool", "healthmonitors": "healthmonitor", "members": "member"}[resource]
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != c.scope || receipt.Endpoint == nil || len(receipt.ResourceRefs) == 0 {
		return nil, types.NewError(types.InvalidArgument)
	}
	cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return nil, e
	}
	if cap.Service != "loadbalancers" || cap.APIGeneration != "v2" || cap.Effect != "mutation" || cap.ContractRevision != receipt.ContractRevision {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if receipt.Outcome != types.Accepted && receipt.Outcome != types.Rejected && receipt.Outcome != types.NotSent && receipt.Outcome != types.Unknown {
		return nil, types.NewError(types.InvalidArgument)
	}
	ep := receipt.Endpoint
	if ep.Scope != c.scope || ep.Audience != "selectel" || ep.APIGeneration != "v2" || ep.ID == "" || types.ValidateEndpoint(ep.URL) != nil {
		return nil, types.NewError(types.ScopeMismatch)
	}
	parts := strings.Split(receipt.OperationID, ".")
	kind := kindFor(parts[2])
	var target *types.ResourceRef
	for i := range receipt.ResourceRefs {
		r := receipt.ResourceRefs[i]
		if e := (service{c.runtime, c.scope}).valid(r, r.Kind); e != nil {
			return nil, e
		}
		if r.EndpointID != ep.ID || r.EndpointURL != ep.URL || kindFor(map[string]string{"loadbalancer": "loadbalancers", "listener": "listeners", "pool": "pools", "healthmonitor": "healthmonitors", "member": "members"}[r.Kind]) == "" {
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
		ref := receipt.ResourceRefs[0]
		return operations.Bind(c.runtime, ref, &receipt, func(context.Context) (operations.Observation, error) {
			return operations.Observation{}, types.NewError(types.RecoveryUnavailable)
		}, nil)
	}
	return c.Handle(*target, &receipt)
}

// Handle observes exact identity. Provisioned means ACTIVE configuration, not traffic health.
func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	s := service{c.runtime, c.scope}
	if kindFor(map[string]string{"loadbalancer": "loadbalancers", "listener": "listeners", "pool": "pools", "healthmonitor": "healthmonitors", "member": "members"}[ref.Kind]) == "" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if e := s.valid(ref, ref.Kind); e != nil {
		return nil, e
	}
	pending := map[string]bool{"PENDING_CREATE": true, "PENDING_UPDATE": true, "PENDING_DELETE": true}
	goals := map[string]operations.Goal{ref.Kind + ".provisioned": {Success: map[string]bool{"ACTIVE": true}, Failure: map[string]bool{"ERROR": true}, Pending: pending}, ref.Kind + ".absent": {AbsentSuccess: true, Pending: map[string]bool{"ACTIVE": true, "ERROR": true, "PENDING_CREATE": true, "PENDING_UPDATE": true, "PENDING_DELETE": true, "DELETED": true}}}
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
		var state string
		var meta types.Metadata
		var e error
		switch ref.Kind {
		case "loadbalancer":
			got, err := c.Get(ctx, ref, types.CallOptions{})
			state = got.Data.ProvisioningStatus
			meta = got.Metadata
			e = err
		case "listener":
			got, err := c.Listeners.Get(ctx, ref, types.CallOptions{})
			state = got.Data.ProvisioningStatus
			meta = got.Metadata
			e = err
		case "pool":
			got, err := c.Pools.Get(ctx, ref, types.CallOptions{})
			state = got.Data.ProvisioningStatus
			meta = got.Metadata
			e = err
		case "healthmonitor":
			got, err := c.Monitors.Get(ctx, ref, types.CallOptions{})
			state = got.Data.ProvisioningStatus
			meta = got.Metadata
			e = err
		case "member":
			got, err := c.Members.Get(ctx, ref, types.CallOptions{})
			state = got.Data.ProvisioningStatus
			meta = got.Metadata
			e = err
		}
		if e != nil {
			return operations.Observation{}, e
		}
		return operations.Observation{Ref: ref, ProviderState: state, ObservedAt: meta.ObservedAt, Metadata: meta}, nil
	}, goals)
}
