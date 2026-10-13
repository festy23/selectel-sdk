// Package operations observes accepted mutations without replaying them.
package operations

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"time"
)

type Observation struct {
	Ref            types.ResourceRef `json:"ref"`
	ProviderState  string            `json:"provider_state"`
	Classification string            `json:"classification"`
	ObservedAt     time.Time         `json:"observed_at"`
	Metadata       types.Metadata    `json:"metadata"`
}
type WaitResult struct {
	Receipt     *types.Receipt `json:"receipt,omitempty"`
	Goal        string         `json:"goal"`
	Last        *Observation   `json:"last,omitempty"`
	Complete    bool           `json:"complete"`
	CompletedAt time.Time      `json:"completed_at"`
}
type RecoveryResult struct {
	Receipt     *types.Receipt `json:"receipt,omitempty"`
	Observation *Observation   `json:"observation,omitempty"`
	ProofStatus string         `json:"proof_status"`
}
type Read func(context.Context) (Observation, error)
type Goal struct {
	Success, Failure, Pending map[string]bool
	AbsentSuccess             bool
}
type Handle struct {
	runtime *core.Runtime
	receipt *types.Receipt
	ref     types.ResourceRef
	read    Read
	goals   map[string]Goal
}

func Bind(r *core.Runtime, ref types.ResourceRef, receipt *types.Receipt, read Read, goals map[string]Goal) (*Handle, error) {
	if r == nil || read == nil || ref.Validate() != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	h := &Handle{runtime: r, ref: cloneRef(ref), receipt: cloneReceipt(receipt), read: read, goals: goals}
	return h, nil
}
func cloneReceipt(r *types.Receipt) *types.Receipt {
	if r == nil {
		return nil
	}
	raw, _ := json.Marshal(r)
	var out types.Receipt
	_ = json.Unmarshal(raw, &out)
	return &out
}
func cloneRef(r types.ResourceRef) types.ResourceRef {
	raw, _ := json.Marshal(r)
	var out types.ResourceRef
	_ = json.Unmarshal(raw, &out)
	return out
}
func (h *Handle) Receipt() *types.Receipt     { return cloneReceipt(h.receipt) }
func (h *Handle) Resource() types.ResourceRef { return cloneRef(h.ref) }
func (h *Handle) check(ctx context.Context) error {
	if ctx == nil {
		return types.NewError(types.InvalidArgument)
	}
	if ctx.Err() != nil {
		return types.ContextError(ctx.Err())
	}
	return nil
}
func (h *Handle) attach(err error) error {
	if e, ok := err.(*types.SDKError); ok {
		copy := *e
		copy.Receipt = h.Receipt()
		return &copy
	}
	return err
}
func (h *Handle) Status(ctx context.Context) (out Observation, err error) {
	defer func() { err = h.attach(err) }()
	if err := h.check(ctx); err != nil {
		return Observation{}, err
	}
	return h.read(ctx)
}
func (h *Handle) Recover(ctx context.Context) (out RecoveryResult, err error) {
	defer func() { err = h.attach(err) }()
	out = RecoveryResult{Receipt: h.Receipt(), ProofStatus: "conclusive"}
	if ctx == nil {
		return out, types.NewError(types.InvalidArgument)
	}
	if h.receipt == nil {
		return out, types.NewError(types.RecoveryUnavailable)
	}
	if h.receipt.Outcome != types.Unknown {
		return out, nil
	}
	out.ProofStatus = "indeterminate"
	if h.receipt.Recovery != "resource" {
		return out, types.NewError(types.RecoveryUnavailable)
	}
	observation, err := h.Status(ctx)
	if types.ErrorCode(err) == types.NotFound {
		observation = Observation{Ref: h.Resource(), Classification: "absent", Metadata: types.Metadata{Scope: h.ref.Scope, EndpointID: h.ref.EndpointID, APIGeneration: h.ref.APIGeneration, ObservedAt: time.Now().UTC()}}
		observation.ObservedAt = observation.Metadata.ObservedAt
		err = nil
	}
	if err != nil {
		return out, err
	}
	out.Observation = &observation
	return out, types.NewError(types.UnknownOutcome)
}
func (h *Handle) Wait(ctx context.Context, goalID string) (out WaitResult, err error) {
	out.Receipt = h.Receipt()
	out.Goal = goalID
	defer func() {
		out.CompletedAt = time.Now().UTC()
		if err != nil {
			if e, ok := err.(*types.SDKError); ok {
				copy := *e
				copy.Receipt = h.Receipt()
				err = &copy
			}
		}
	}()
	if ctx == nil || goalID == "" {
		return out, types.NewError(types.InvalidArgument)
	}
	goal, ok := h.goals[goalID]
	if !ok {
		return out, types.NewError(types.CapabilityUnavailable)
	}
	timeout, poll := h.runtime.WaitSettings()
	call, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if h.receipt != nil {
		switch h.receipt.Outcome {
		case types.Unknown:
			recovery, recoveryErr := h.Recover(call)
			out.Last = recovery.Observation
			return out, recoveryErr
		case types.NotSent, types.Rejected:
			e := types.NewError(types.CapabilityUnavailable)
			e.Reason = "mutation_not_accepted"
			return out, e
		case types.Accepted:
		default:
			return out, types.NewError(types.InvalidArgument)
		}
	}
	for {
		observation, statusErr := h.Status(call)
		if statusErr != nil {
			if types.ErrorCode(statusErr) == types.NotFound && goal.AbsentSuccess {
				observation = Observation{Ref: h.Resource(), Classification: "absent", Metadata: types.Metadata{Scope: h.ref.Scope, EndpointID: h.ref.EndpointID, APIGeneration: h.ref.APIGeneration, ObservedAt: time.Now().UTC()}}
				observation.ObservedAt = observation.Metadata.ObservedAt
				out.Last = &observation
				out.Complete = true
				return out, nil
			}
			return out, statusErr
		}
		out.Last = &observation
		if goal.Success[observation.ProviderState] {
			out.Complete = true
			return out, nil
		}
		if goal.Failure[observation.ProviderState] {
			return out, types.NewError(types.OperationFailed)
		}
		if !goal.Pending[observation.ProviderState] {
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
