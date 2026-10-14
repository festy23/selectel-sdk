package images

import (
	"context"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
	"strings"
)

func states(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, s := range v {
		m[s] = true
	}
	return m
}

// Handle preserves the caller's exact authorization view, never the image owner.
func (c *Client) Handle(r types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if r.Kind != "image" && r.Kind != "image_member" {
		return nil, types.NewError(types.CapabilityUnavailable)
	}
	if e := validateRef(r, r.Kind); e != nil {
		return nil, e
	}
	if r.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if receipt != nil {
		cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		expected := "images.v2.images."
		if r.Kind == "image_member" {
			expected = "images.v2.members."
		}
		if !strings.HasPrefix(receipt.OperationID, expected) || cap.Effect != "mutation" || cap.Service != "images" || cap.APIGeneration != "v2" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if cap.ContractRevision != receipt.ContractRevision {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.Scope != r.Scope || receipt.Endpoint == nil || *receipt.Endpoint != *pin(r) || len(receipt.ResourceRefs) != 1 || !reflect.DeepEqual(receipt.ResourceRefs[0], r) || receipt.ProviderOperationRef != nil || receipt.Recovery != "resource" || receipt.Correlation != "" {
			return nil, invalid()
		}
		switch receipt.Outcome {
		case types.Accepted, types.Rejected, types.NotSent, types.Unknown:
		default:
			return nil, invalid()
		}
	}
	if r.Kind == "image_member" {
		read := func(ctx context.Context) (operations.Observation, error) {
			got, e := c.Members.Get(ctx, r, types.CallOptions{})
			if e != nil {
				return operations.Observation{}, e
			}
			class := "unknown"
			switch got.Data.Status {
			case "accepted":
				class = "succeeded"
			case "pending":
				class = "pending"
			case "rejected":
				class = "failed"
			}
			return operations.Observation{Ref: r, ProviderState: got.Data.Status, Classification: class, ObservedAt: got.Metadata.ObservedAt, Metadata: got.Metadata}, nil
		}
		return operations.Bind(c.runtime, r, receipt, read, map[string]operations.Goal{"member.accepted": {Success: states("accepted"), Pending: states("pending"), Failure: states("rejected")}, "member.absent": {Pending: states("pending", "accepted", "rejected"), AbsentSuccess: true}})
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		got, e := c.Get(ctx, r, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		class := "unknown"
		switch got.Data.Status {
		case "active":
			class = "succeeded"
		case "queued", "saving", "importing", "uploading", "pending_delete", "deactivated":
			class = "pending"
		case "killed", "deleted":
			class = "failed"
		}
		return operations.Observation{Ref: r, ProviderState: got.Data.Status, Classification: class, ObservedAt: got.Metadata.ObservedAt, Metadata: got.Metadata}, nil
	}
	return operations.Bind(c.runtime, r, receipt, read, map[string]operations.Goal{"image.active": {Success: states("active"), Pending: states("queued", "saving", "importing", "uploading", "deactivated"), Failure: states("killed", "deleted", "pending_delete")}, "image.absent": {Pending: states("active", "queued", "saving", "importing", "uploading", "deactivated", "killed", "pending_delete", "deleted"), AbsentSuccess: true}})
}
