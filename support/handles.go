package support

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
	"strconv"
)

func (c *Client) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	var runtime *core.Runtime
	var read operations.Read
	goals := map[string]operations.Goal{}
	switch ref.APIGeneration {
	case "attachments_v1":
		client := c.Attachments
		runtime = client.runtime
		if e := client.refError(ref); e != nil {
			return nil, e
		}
		goals["file_metadata_observed"] = operations.Goal{Success: map[string]bool{"present": true}}
		read = func(ctx context.Context) (operations.Observation, error) {
			out, e := client.GetMetadata(ctx, ref, types.CallOptions{})
			state, class := "", "unknown"
			if out.Data.FileUUID != nil {
				state, class = "present", "succeeded"
			}
			return operations.Observation{Ref: ref, ProviderState: state, Classification: class, Metadata: out.Metadata, ObservedAt: out.Metadata.ObservedAt}, e
		}
	case "v2":
		client := c.Tickets
		runtime = client.runtime
		if e := client.refError(ref); e != nil {
			return nil, e
		}
		known := map[string]bool{"in_progress": true, "resolved": true, "closed": true, "pending": true, "replied": true, "new": true, "awaiting": true}
		opened := map[string]bool{}
		for state := range known {
			if state != "closed" {
				opened[state] = true
			}
		}
		goals["ticket_observed"] = operations.Goal{Success: known}
		goals["ticket_closed"] = operations.Goal{Success: map[string]bool{"closed": true}, Pending: opened}
		goals["ticket_not_closed"] = operations.Goal{Success: opened, Pending: map[string]bool{"closed": true}}
		read = func(ctx context.Context) (operations.Observation, error) {
			out, e := client.Get(ctx, ref, types.CallOptions{})
			state, class := "", "unknown"
			if out.Data.Status != nil {
				state = string(*out.Data.Status)
			}
			if known[state] {
				class = "pending"
			}
			if state == "closed" {
				class = "succeeded"
			}
			return operations.Observation{Ref: ref, ProviderState: state, Classification: class, Metadata: out.Metadata, ObservedAt: out.Metadata.ObservedAt}, e
		}
	case "v1":
		client := c.Legacy
		runtime = client.runtime
		if e := client.refError(ref, ref.Kind); e != nil {
			return nil, e
		}
		if ref.Kind == "comment" {
			goals["comment_observed"] = operations.Goal{Success: map[string]bool{"present": true}}
			read = func(ctx context.Context) (operations.Observation, error) {
				out, e := client.GetComment(ctx, ref, types.CallOptions{})
				state, class := "", "unknown"
				if out.Data.ID != nil {
					state, class = "present", "succeeded"
				}
				return operations.Observation{Ref: ref, ProviderState: state, Classification: class, Metadata: out.Metadata, ObservedAt: out.Metadata.ObservedAt}, e
			}
		} else if ref.Kind == "ticket" {
			goals["ticket_observed"] = operations.Goal{Success: map[string]bool{"true": true, "false": true}}
			goals["ticket_closed"] = operations.Goal{Success: map[string]bool{"true": true}, Pending: map[string]bool{"false": true}}
			goals["ticket_not_closed"] = operations.Goal{Success: map[string]bool{"false": true}, Pending: map[string]bool{"true": true}}
			read = func(ctx context.Context) (operations.Observation, error) {
				out, e := client.Get(ctx, ref, types.CallOptions{})
				state, class := "", "unknown"
				if out.Data.IsClosed != nil {
					state = strconv.FormatBool(*out.Data.IsClosed)
					class = "pending"
					if *out.Data.IsClosed {
						class = "succeeded"
					}
				}
				return operations.Observation{Ref: ref, ProviderState: state, Classification: class, Metadata: out.Metadata, ObservedAt: out.Metadata.ObservedAt}, e
			}
		} else {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
	default:
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if receipt != nil {
		selected, e := c.receiptRef(*receipt)
		if e != nil {
			return nil, e
		}
		if !reflect.DeepEqual(selected, ref) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		capability, e := runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		allowed := map[string]operations.Goal{}
		for _, id := range capability.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	return operations.Bind(runtime, ref, receipt, read, goals)
}
func (c *Client) receiptRef(receipt types.Receipt) (types.ResourceRef, error) {
	runtime, scope := c.Tickets.runtime, c.Tickets.scope
	bad := func() (types.ResourceRef, error) { return types.ResourceRef{}, types.NewError(types.InvalidArgument) }
	if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != scope || receipt.Endpoint == nil || receipt.Endpoint.Scope != scope || receipt.Endpoint.Audience != "selectel" || receipt.ProviderOperationRef != nil {
		return bad()
	}
	switch receipt.Outcome {
	case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
	default:
		return bad()
	}
	capability, e := runtime.Catalog().Describe(receipt.OperationID)
	if e != nil {
		return types.ResourceRef{}, e
	}
	if capability.ContractRevision != receipt.ContractRevision {
		return types.ResourceRef{}, types.NewError(types.CapabilityUnavailable)
	}
	if capability.Service != "tickets" || capability.Effect != "mutation" || capability.APIGeneration != receipt.Endpoint.APIGeneration {
		return bad()
	}
	expected := "ticket"
	switch receipt.OperationID {
	case "tickets.attachments_v1.files.upload":
		expected = "file"
	case "tickets.v1.comments.create", "tickets.v1.comments.vote":
		expected = "comment"
	case "tickets.v2.tickets.create", "tickets.v2.tickets.update", "tickets.v1.tickets.create", "tickets.v1.tickets.update", "tickets.v1.tickets.close", "tickets.v1.tickets.reopen", "tickets.v1.tickets.vote":
	default:
		return types.ResourceRef{}, types.NewError(types.CapabilityUnavailable)
	}
	var selected *types.ResourceRef
	for i := range receipt.ResourceRefs {
		ref := receipt.ResourceRefs[i]
		if ref.Validate() != nil || ref.Scope != scope || ref.Service != "tickets" || ref.APIGeneration != capability.APIGeneration || ref.EndpointID != receipt.Endpoint.ID || ref.EndpointURL != receipt.Endpoint.URL {
			return bad()
		}
		if ref.Kind == expected {
			if selected != nil {
				return bad()
			}
			copy := ref
			selected = &copy
		}
	}
	if selected == nil {
		return types.ResourceRef{}, types.NewError(types.RecoveryUnavailable)
	}
	if receipt.OperationID == "tickets.v1.comments.create" {
		if len(receipt.ResourceRefs) != 2 || selected.Parent == nil {
			return bad()
		}
		anchorFound := false
		for _, ref := range receipt.ResourceRefs {
			if reflect.DeepEqual(ref, *selected.Parent) {
				anchorFound = true
			}
		}
		if !anchorFound {
			return bad()
		}
	} else if len(receipt.ResourceRefs) != 1 {
		return bad()
	}
	return *selected, nil
}
func (c *Client) Resume(receipt types.Receipt) (*operations.Handle, error) {
	ref, e := c.receiptRef(receipt)
	if e != nil {
		return nil, e
	}
	return c.Handle(ref, &receipt)
}
