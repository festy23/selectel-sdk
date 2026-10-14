package dedicated

import (
	"context"
	"reflect"
	"strings"

	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
)

func observe(ref types.ResourceRef, state, classification string, metadata types.Metadata) operations.Observation {
	return operations.Observation{Ref: cloneRef(ref), ProviderState: state, Classification: classification, ObservedAt: metadata.ObservedAt, Metadata: metadata}
}
func (c *Client) handle(ref types.ResourceRef, receipt *types.Receipt, read operations.Read, goals map[string]operations.Goal) (*operations.Handle, error) {
	if e := c.refError(ref, "resource"); e != nil {
		return nil, e
	}
	if receipt != nil {
		capability, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if !strings.HasPrefix(receipt.OperationID, "dedicated.v2.") || capability.Effect != "mutation" || receipt.ContractRevision != capability.ContractRevision || receipt.Scope != c.scope || receipt.Endpoint == nil || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.Scope != c.scope || receipt.Endpoint.APIGeneration != "v2" {
			return nil, types.NewError(types.ScopeMismatch)
		}
		known := false
		for _, r := range receipt.ResourceRefs {
			if reflect.DeepEqual(r, ref) {
				known = true
			}
		}
		if !known {
			return nil, types.NewError(types.ScopeMismatch)
		}
		allowed := map[string]operations.Goal{}
		for _, id := range capability.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	return operations.Bind(c.runtime, ref, receipt, read, goals)
}

// PowerHandle observes stable physical power only; it does not prove a reboot
// cycle, installation completion or application readiness.
func (c *Client) PowerHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refError(ref, "resource"); e != nil {
		return nil, e
	}
	ref = cloneRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.GetServerPowerState(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		if v.Data.Data == nil || v.Data.Data.Result == nil || v.Data.Data.Result.DriverStatus == nil || v.Data.Data.Result.DriverStatus.PowerState == nil {
			return operations.Observation{}, types.NewError(types.ProtocolError)
		}
		power := v.Data.Data.Result.DriverStatus
		state := *power.PowerState
		if power.TargetPowerState == nil {
			return operations.Observation{}, types.NewError(types.ProtocolError)
		}
		if *power.TargetPowerState != "" {
			state += "->" + *power.TargetPowerState
		}
		classification := "unknown"
		switch state {
		case "power on", "power off":
			classification = "succeeded"
		case "power off->power on", "power on->power off", "power on->power on":
			classification = "pending"
		}
		return observe(ref, state, classification, v.Metadata), nil
	}
	pending := map[string]bool{"power off": true, "power on": true, "power off->power on": true, "power on->power off": true, "power on->power on": true}
	return c.handle(ref, receipt, read, map[string]operations.Goal{"dedicated.power.on": {Success: map[string]bool{"power on": true}, Pending: pending}, "dedicated.power.off": {Success: map[string]bool{"power off": true}, Pending: pending}})
}
func (c *Client) ResourceHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refError(ref, "resource"); e != nil {
		return nil, e
	}
	ref = cloneRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.GetResourceDetails(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		state := "task_queued"
		if v.Data.Queued == nil {
			if v.Data.Data == nil || v.Data.Data.Result == nil || v.Data.Data.Result.State == nil {
				return operations.Observation{}, types.NewError(types.ProtocolError)
			}
			state = *v.Data.Data.Result.State
		}
		classification := "unknown"
		switch state {
		case "active", "expiring", "ending":
			classification = "succeeded"
		case "task_queued", "pending", "processing", "paid", "deploy":
			classification = "pending"
		case "blocked", "cancel", "deleted":
			classification = "failed"
		}
		return observe(ref, state, classification, v.Metadata), nil
	}
	pending := map[string]bool{"task_queued": true, "pending": true, "processing": true, "paid": true, "deploy": true}
	failure := map[string]bool{"blocked": true, "cancel": true, "deleted": true}
	nonRenewPending := map[string]bool{"task_queued": true, "pending": true, "processing": true, "paid": true, "deploy": true, "active": true}
	return c.handle(ref, receipt, read, map[string]operations.Goal{"dedicated.resource.active": {Success: map[string]bool{"active": true}, Pending: pending, Failure: failure}, "dedicated.resource.non_renewing": {Success: map[string]bool{"expiring": true, "ending": true}, Pending: nonRenewPending, Failure: failure}})
}

// OSHandle confirms only the observed installation flags, never host health or
// authorship of an unknown write. Unknown receipts stay indeterminate.
func (c *Client) OSHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := c.refError(ref, "resource"); e != nil {
		return nil, e
	}
	ref = cloneRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.GetServerOSDetails(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		if v.Data.Data == nil || v.Data.Data.Result == nil || v.Data.Data.Result.Reinstall == nil || v.Data.Data.Result.IsProcessing == nil {
			return operations.Observation{}, types.NewError(types.ProtocolError)
		}
		state := v.Data.Data.Result.Reinstall.String()
		if state == "0" && *v.Data.Data.Result.IsProcessing {
			state = "processing"
		}
		classification := "unknown"
		switch state {
		case "0":
			classification = "succeeded"
		case "1", "processing":
			classification = "pending"
		}
		return observe(ref, state, classification, v.Metadata), nil
	}
	return c.handle(ref, receipt, read, map[string]operations.Goal{"dedicated.os.installation_finished": {Success: map[string]bool{"0": true}, Pending: map[string]bool{"1": true, "processing": true}}})
}
