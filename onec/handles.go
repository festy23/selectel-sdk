package onec

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/types"
	"reflect"
)

var clusterPending = map[string]bool{
	"CLUSTER_STATUS_PENDING": true, "CLUSTER_STATUS_CREATING": true,
	"CLUSTER_STATUS_RESIZING": true, "CLUSTER_STATUS_RESTARTING": true,
	"CLUSTER_STATUS_UPGRADING": true, "CLUSTER_STATUS_MAINTENANCE": true,
	"CLUSTER_STATUS_DELETING": true, "CLUSTER_STATUS_BEING_BLOCKED": true,
	"CLUSTER_STATUS_BLOCKED": true, "CLUSTER_STATUS_BEING_UNBLOCKED": true,
	"CLUSTER_STATUS_MODIFYING": true,
}

// ClusterHandle observes only this exact cluster and never replays a mutation.
func (c *Client) ClusterHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "cluster" || ref.Parent != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return nil, err
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return nil, types.NewError(types.EndpointUnavailable)
	}
	activePending := map[string]bool{}
	deletedPending := map[string]bool{"CLUSTER_STATUS_ACTIVE": true}
	for s := range clusterPending {
		activePending[s] = true
		deletedPending[s] = true
	}
	goals := map[string]operations.Goal{
		"cluster.active":  {Success: map[string]bool{"CLUSTER_STATUS_ACTIVE": true}, Failure: map[string]bool{"CLUSTER_STATUS_ERRORED": true, "CLUSTER_STATUS_DELETED": true}, Pending: activePending},
		"cluster.deleted": {Success: map[string]bool{"CLUSTER_STATUS_DELETED": true}, Failure: map[string]bool{"CLUSTER_STATUS_ERRORED": true}, Pending: deletedPending},
	}
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receiptResourceKind(receipt.OperationID) != "cluster" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if cap.ContractRevision != receipt.ContractRevision || cap.Service != "cloud_1c" || cap.APIGeneration != "v2" || cap.Effect != "mutation" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.Scope != ref.Scope || receipt.Endpoint == nil || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Audience != "selectel" || len(receipt.ResourceRefs) != 1 || !reflect.DeepEqual(receipt.ResourceRefs[0], ref) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		allowed := map[string]operations.Goal{}
		for _, id := range cap.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := c.GetCluster(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		state := ""
		class := "unknown"
		if result.Data.Cluster.State != nil && result.Data.Cluster.State.Status != nil {
			state = *result.Data.Cluster.State.Status
		}
		switch state {
		case "CLUSTER_STATUS_ACTIVE":
			class = "succeeded"
		case "CLUSTER_STATUS_DELETED":
			class = "absent"
		case "CLUSTER_STATUS_ERRORED":
			class = "failed"
		default:
			if clusterPending[state] {
				class = "pending"
			}
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	return operations.Bind(c.runtime, ref, receipt, read, goals)
}

var datastorePending = map[string]bool{"DATASTORE_STATUS_PENDING": true, "DATASTORE_STATUS_CREATING": true, "DATASTORE_STATUS_RESIZING": true, "DATASTORE_STATUS_DELETING": true, "DATASTORE_STATUS_MAINTENANCE": true}

func (c *Client) DatastoreHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "datastore" || ref.Parent != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return nil, err
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return nil, types.NewError(types.EndpointUnavailable)
	}
	activePending := map[string]bool{}
	deletedPending := map[string]bool{"DATASTORE_STATUS_ACTIVE": true}
	for s := range datastorePending {
		activePending[s] = true
		deletedPending[s] = true
	}
	goals := map[string]operations.Goal{
		"datastore.active":  {Success: map[string]bool{"DATASTORE_STATUS_ACTIVE": true}, Failure: map[string]bool{"DATASTORE_STATUS_ERRORED": true, "DATASTORE_STATUS_DELETED": true}, Pending: activePending},
		"datastore.deleted": {Success: map[string]bool{"DATASTORE_STATUS_DELETED": true}, Failure: map[string]bool{"DATASTORE_STATUS_ERRORED": true}, Pending: deletedPending},
	}
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receiptResourceKind(receipt.OperationID) != "datastore" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if cap.ContractRevision != receipt.ContractRevision || cap.Service != "cloud_1c" || cap.APIGeneration != "v2" || cap.Effect != "mutation" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.Scope != ref.Scope || receipt.Endpoint == nil || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Audience != "selectel" || len(receipt.ResourceRefs) != 1 || !reflect.DeepEqual(receipt.ResourceRefs[0], ref) {
			return nil, types.NewError(types.ScopeMismatch)
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		allowed := map[string]operations.Goal{}
		for _, id := range cap.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := c.GetDatastore(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		state := ""
		class := "unknown"
		if result.Data.Datastore.State != nil && result.Data.Datastore.State.Status != nil {
			state = *result.Data.Datastore.State.Status
		}
		switch state {
		case "DATASTORE_STATUS_ACTIVE":
			class = "succeeded"
		case "DATASTORE_STATUS_DELETED":
			class = "absent"
		case "DATASTORE_STATUS_ERRORED":
			class = "failed"
		default:
			if datastorePending[state] {
				class = "pending"
			}
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	return operations.Bind(c.runtime, ref, receipt, read, goals)
}

// Only compiled originating mutations may attach a receipt to a resource kind.
func receiptResourceKind(operationID string) string {
	switch operationID {
	case "cloud_1c.v2.clusters.create", "cloud_1c.v2.clusters.delete", "cloud_1c.v2.clusters.update", "cloud_1c.v2.clusters_config.create", "cloud_1c.v2.clusters_restart.create", "cloud_1c.v2.clusters_resize.create", "cloud_1c.v2.clusters_upgrade.create":
		return "cluster"
	case "cloud_1c.v2.datastores.create", "cloud_1c.v2.datastores.delete", "cloud_1c.v2.datastores.update", "cloud_1c.v2.datastores_resize.create":
		return "datastore"
	case "cloud_1c.v2.cluster_proxies.create", "cloud_1c.v2.cluster_proxies.delete":
		return "cluster_proxy"
	default:
		return ""
	}
}

var proxyPending = map[string]bool{"CLUSTER_PROXY_STATUS_PENDING": true, "CLUSTER_PROXY_STATUS_CREATING": true, "CLUSTER_PROXY_STATUS_DELETING": true, "CLUSTER_PROXY_STATUS_UPDATING": true}

func (c *Client) ClusterProxyHandle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := ref.Validate(); err != nil {
		return nil, err
	}
	if ref.Service != "cloud_1c" || ref.APIGeneration != "v2" || ref.Scope != c.scope {
		return nil, types.NewError(types.ScopeMismatch)
	}
	if ref.Kind != "cluster_proxy" {
		return nil, types.NewError(types.InvalidArgument)
	}
	op := core.Operation{Service: "cloud_1c", APIGeneration: "v2", RequireProject: true, RequireLocation: true}
	op, err := c.runtime.BindDocumented(op, c.scope)
	if err != nil {
		return nil, err
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return nil, types.NewError(types.EndpointUnavailable)
	}
	if e := c.validateProxyRef(ref, op); e != nil {
		return nil, e
	}
	ref.Parent = cloneResourceRef(ref.Parent)
	activePending := map[string]bool{}
	deletedPending := map[string]bool{"CLUSTER_PROXY_STATUS_ACTIVE": true}
	for s := range proxyPending {
		activePending[s] = true
		deletedPending[s] = true
	}
	goals := map[string]operations.Goal{
		"cluster_proxy.active":  {Success: map[string]bool{"CLUSTER_PROXY_STATUS_ACTIVE": true}, Failure: map[string]bool{"CLUSTER_PROXY_STATUS_ERRORED": true, "CLUSTER_PROXY_STATUS_DELETED": true}, Pending: activePending},
		"cluster_proxy.deleted": {Success: map[string]bool{"CLUSTER_PROXY_STATUS_DELETED": true}, Failure: map[string]bool{"CLUSTER_PROXY_STATUS_ERRORED": true}, Pending: deletedPending},
	}
	if receipt != nil {
		if receipt.SchemaVersion != 1 || receipt.CallID == "" || receipt.OperationID == "" || receipt.ContractRevision == "" {
			return nil, types.NewError(types.InvalidArgument)
		}
		if receipt.OperationID == "cloud_1c.v2.cluster_proxies.create" && ref.Parent == nil {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if receiptResourceKind(receipt.OperationID) != "cluster_proxy" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.ProviderOperationRef != nil {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		cap, e := c.runtime.Catalog().Describe(receipt.OperationID)
		if e != nil {
			return nil, e
		}
		if cap.ContractRevision != receipt.ContractRevision || cap.Service != "cloud_1c" || cap.APIGeneration != "v2" || cap.Effect != "mutation" {
			return nil, types.NewError(types.CapabilityUnavailable)
		}
		if receipt.Scope != ref.Scope || receipt.Endpoint == nil || receipt.Endpoint.Scope != ref.Scope || receipt.Endpoint.ID != ref.EndpointID || receipt.Endpoint.URL != ref.EndpointURL || receipt.Endpoint.APIGeneration != "v2" || receipt.Endpoint.Audience != "selectel" {
			return nil, types.NewError(types.ScopeMismatch)
		}
		if e := validateReceiptReferenceSet(*receipt, ref); e != nil {
			return nil, e
		}
		switch receipt.Outcome {
		case types.NotSent, types.Rejected, types.Accepted, types.Unknown:
		default:
			return nil, types.NewError(types.InvalidArgument)
		}
		allowed := map[string]operations.Goal{}
		for _, id := range cap.WaitGoals {
			if goal, ok := goals[id]; ok {
				allowed[id] = goal
			}
		}
		goals = allowed
	}
	read := func(ctx context.Context) (operations.Observation, error) {
		result, e := c.GetClusterProxy(ctx, ref, types.CallOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		state := ""
		class := "unknown"
		if result.Data.ClusterProxy.State != nil && result.Data.ClusterProxy.State.Status != nil {
			state = *result.Data.ClusterProxy.State.Status
		}
		switch state {
		case "CLUSTER_PROXY_STATUS_ACTIVE":
			class = "succeeded"
		case "CLUSTER_PROXY_STATUS_DELETED":
			class = "absent"
		case "CLUSTER_PROXY_STATUS_ERRORED":
			class = "failed"
		default:
			if proxyPending[state] {
				class = "pending"
			}
		}
		return operations.Observation{Ref: ref, ProviderState: state, Classification: class, ObservedAt: result.Metadata.ObservedAt, Metadata: result.Metadata}, nil
	}
	return operations.Bind(c.runtime, ref, receipt, read, goals)
}
