package volumes

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
	"time"
)

func states(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// Handle observes this exact reference. A scoped 404 is confirmed against a
// complete scoped list before it can satisfy volume.absent; it is not global proof.
func (c *Client) Handle(r types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if validateRef(r, "volume") != nil || r.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if receipt != nil {
		switch receipt.OperationID {
		case "volumes.v3.volumes.create", "volumes.v3.volumes.update", "volumes.v3.volumes.extend", "volumes.v3.volumes.delete":
		default:
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if capability.ContractRevision != receipt.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != r.Scope || receipt.Endpoint == nil || *receipt.Endpoint != *pin(r) || len(receipt.ResourceRefs) != 1 || !reflect.DeepEqual(receipt.ResourceRefs[0], r) || receipt.ProviderOperationRef != nil || receipt.Correlation != "" || receipt.Recovery != "resource" {
			return nil, invalid()
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, invalid()
		}
	}
	pending := states("creating", "downloading", "restoring-backup", "extending", "in-use", "deleting")
	failures := states("error", "error_deleting", "error_extending", "error_restoring", "error_backing-up", "error_managing", "error_unmanaging")
	read := func(ctx context.Context) (operations.Observation, error) {
		got, e := c.Get(ctx, r, types.CallOptions{})
		if types.ErrorCode(e) == types.NotFound {
			exact := *c
			exact.listPin = pin(r)
			all, listErr := exact.Collect(ctx, pagination.Options{})
			if listErr != nil {
				return operations.Observation{}, listErr
			}
			if !all.Complete {
				return operations.Observation{}, types.NewError(types.Incomplete)
			}
			for _, v := range all.Items {
				if v.ID == r.ID {
					return operations.Observation{}, types.NewError(types.PermissionDenied)
				}
			}
		}
		if e != nil {
			return operations.Observation{}, e
		}
		state := got.Data.Status
		class := "unknown"
		if state == "available" {
			class = "succeeded"
		} else if pending[state] {
			class = "pending"
		} else if failures[state] {
			class = "failed"
		}
		return operations.Observation{Ref: r, ProviderState: state, Classification: class, ObservedAt: got.Metadata.ObservedAt, Metadata: got.Metadata}, nil
	}
	return operations.Bind(c.runtime, r, receipt, read, map[string]operations.Goal{"volume.available": {Success: states("available"), Pending: pending, Failure: failures}, "volume.absent": {Pending: func() map[string]bool {
		m := states("available")
		for s := range pending {
			m[s] = true
		}
		return m
	}(), Failure: failures, AbsentSuccess: true}})
}

type ExtendWaitResult struct {
	Last     *types.Result[Volume] `json:"last,omitempty"`
	Complete bool                  `json:"complete"`
}

// WaitExtended observes a previously selected volume; it never sends Extend.
// Attached explicitly permits in-use as terminal, and size must match exactly.
func (c *Client) WaitExtended(ctx context.Context, r types.ResourceRef, size types.Integer, attached bool) (out ExtendWaitResult, err error) {
	if ctx == nil || !positive(size) || validateRef(r, "volume") != nil {
		return out, invalid()
	}
	if r.Scope != c.scope {
		return out, types.NewError(types.ScopeMismatch)
	}
	timeout, poll := c.runtime.WaitSettings()
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		got, e := c.Get(call, r, types.CallOptions{})
		if e != nil {
			return out, e
		}
		out.Last = &got
		switch got.Data.Status {
		case "available", "in-use":
			// A just-accepted extend may still expose the previous stable state.
			// Only the exact target and explicitly allowed attachment state finish.
			if got.Data.Size.String() == size.String() && (got.Data.Status == "available" || attached) {
				out.Complete = true
				return out, nil
			}
		case "creating", "downloading", "restoring-backup", "extending":
		case "error", "error_extending", "error_deleting", "error_restoring":
			return out, types.NewError(types.OperationFailed)
		default:
			return out, types.NewError(types.StateUnknown)
		}
		timer := time.NewTimer(poll)
		select {
		case <-call.Done():
			timer.Stop()
			return out, types.ContextError(call.Err())
		case <-timer.C:
		}
	}
}
