package cdn

import (
	"context"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) CacheTask(id string) (types.ResourceRef, error) {
	if !opaqueID(id) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("cdn.v3.cache_tasks.get", "GET")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: "v3", Scope: c.scope}, "cdn", "cache_task", id), nil
}
func stateClass(state string, success, failure, pending map[string]bool) string {
	if success[state] {
		return "succeeded"
	}
	if failure[state] {
		return "failed"
	}
	if pending[state] {
		return "pending"
	}
	return "unknown"
}
func observe(ref types.ResourceRef, state string, metadata types.Metadata, goal operations.Goal) operations.Observation {
	return operations.Observation{Ref: ref, ProviderState: state, Classification: stateClass(state, goal.Success, goal.Failure, goal.Pending), ObservedAt: metadata.ObservedAt, Metadata: metadata}
}
func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refError(ref, ref.Kind); e != nil {
		return nil, e
	}
	order := false
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Endpoint == nil || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.APIGeneration != "v3" || receipt.Endpoint.Audience != "selectel" || receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.InvalidArgument)
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if capability.Effect != "mutation" || capability.Service != "cdn" || capability.APIGeneration != "v3" || capability.ContractRevision != receipt.ContractRevision || receipt.Scope != c.scope || len(receipt.ResourceRefs) != 1 || receipt.ResourceRefs[0] != ref {
			return nil, types.NewError(types.InvalidArgument)
		}
		expected := "resource"
		switch receipt.OperationID {
		case "cdn.v3.cache_tasks.create":
			expected = "cache_task"
		case "cdn.v3.certificates.create", "cdn.v3.certificates.update", "cdn.v3.certificates.delete":
			expected = "certificate"
		case "cdn.v3.letsencrypt.order":
			order = true
		case "cdn.v3.resources.create", "cdn.v3.resources.update", "cdn.v3.resources.replace":
		default:
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if ref.Kind != expected {
			return nil, types.NewError(types.InvalidArgument)
		}
	}
	if order {
		return c.orderHandle(ref, receipt)
	}
	var read operations.Read
	goals := map[string]operations.Goal{}
	switch ref.Kind {
	case "cache_task":
		goal := operations.Goal{Success: map[string]bool{"ok": true}, Failure: map[string]bool{"failed": true}, Pending: map[string]bool{"in_queue": true, "processing": true}}
		goals["cache_processed"] = goal
		read = func(ctx context.Context) (operations.Observation, error) {
			out, e := c.GetCacheTask(ctx, ref, types.CallOptions{})
			state := ""
			if out.Data.Status != nil {
				state = *out.Data.Status
			}
			return observe(ref, state, out.Metadata, goal), e
		}
	case "resource":
		goal := operations.Goal{Success: map[string]bool{"Completed": true}, Failure: map[string]bool{"Error": true}, Pending: map[string]bool{"Processing": true}}
		goals["configuration_observed"] = goal
		read = func(ctx context.Context) (operations.Observation, error) {
			out, e := c.GetResourceStatus(ctx, ref, types.CallOptions{})
			state := ""
			if out.Data.Status != nil {
				state = *out.Data.Status
			}
			return observe(ref, state, out.Metadata, goal), e
		}
	case "certificate":
		goal := operations.Goal{Success: map[string]bool{"0": true}}
		goals["certificate_present"] = goal
		goals["certificate_absent"] = operations.Goal{AbsentSuccess: true, Pending: map[string]bool{"0": true}}
		read = func(ctx context.Context) (operations.Observation, error) {
			out, e := c.GetCertificate(ctx, ref, CertificateOptions{}, types.CallOptions{})
			state := ""
			if out.Data.Status != nil {
				state = out.Data.Status.String()
			}
			observation := observe(ref, state, out.Metadata, goal)
			if e != nil {
				return observation, e
			}
			if out.Data.Data == nil || len(*out.Data.Data) == 0 {
				observation.Classification = "unknown"
				return observation, types.NewError(types.StateUnknown)
			}
			return observation, nil
		}
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	return operations.Bind(c.runtime, ref, receipt, read, goals)
}
func (c *Client) LetsEncryptHandle(ref types.ResourceRef) (*operations.Handle, error) {
	if e := c.refError(ref, "resource"); e != nil {
		return nil, e
	}
	return c.orderHandle(ref, nil)
}
func (c *Client) orderHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	goal := operations.Goal{Success: map[string]bool{"processed": true}, Failure: map[string]bool{"failed": true}, Pending: map[string]bool{"accepted": true}}
	return operations.Bind(c.runtime, ref, receipt, func(ctx context.Context) (operations.Observation, error) {
		out, e := c.GetLetsEncryptStatus(ctx, ref, types.CallOptions{})
		state := ""
		if out.Data.Data != nil && out.Data.Data.TaskStatus != nil {
			state = *out.Data.Data.TaskStatus
		}
		return observe(ref, state, out.Metadata, goal), e
	}, map[string]operations.Goal{"certificate_order_processed": goal})
}
