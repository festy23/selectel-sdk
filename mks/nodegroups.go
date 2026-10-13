package mks

import (
	"context"
	"encoding/base64"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Nodegroups struct{ service }
type Nodes struct{ service }
type NodegroupListOptions struct {
	Cluster types.ResourceRef `json:"cluster"`
}

func (c *Nodegroups) Iterate(cluster types.ResourceRef, o pagination.Options) (*pagination.Iterator[NodegroupListItem], error) {
	if err := validateRef(cluster, "cluster", c.scope); err != nil {
		return nil, err
	}
	op, err := c.runtime.BindDocumented(core.Operation{ID: "mks.v2.clusters_nodegroups.list", Service: "mks", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, c.scope)
	if err != nil {
		return nil, err
	}
	return core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: resourcePath(cluster) + "/nodegroups", Refs: []types.ResourceRef{cluster}, PinnedEndpoint: pinned(cluster)}, NodegroupListOptions{Cluster: cluster}, o, "nodegroups", func(v *NodegroupListItem, endpoint types.EndpointBinding) error {
		if err := v.Validate(); err != nil {
			return err
		}
		if *v.ClusterID != cluster.ID || *v.ID == "" {
			return types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(endpoint, "mks", "nodegroup", *v.ID)
		v.Ref.Parent = &cluster
		return attachNodes(v.Nodes, *v.Ref, endpoint)
	})
}
func (c *Nodegroups) ListPage(ctx context.Context, cluster types.ResourceRef, o pagination.Options) (pagination.Page[NodegroupListItem], error) {
	it, e := c.Iterate(cluster, o)
	if e != nil {
		return pagination.Page[NodegroupListItem]{}, e
	}
	return it.Next(ctx)
}
func (c *Nodegroups) Collect(ctx context.Context, cluster types.ResourceRef, o pagination.Options) (pagination.Collection[NodegroupListItem], error) {
	it, e := c.Iterate(cluster, o)
	if e != nil {
		return pagination.Collection[NodegroupListItem]{Items: []NodegroupListItem{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func attachNodes(nodes []Node, parent types.ResourceRef, endpoint types.EndpointBinding) error {
	for i := range nodes {
		if nodes[i].NodegroupID == nil || *nodes[i].NodegroupID != parent.ID {
			return types.NewError(types.ScopeMismatch)
		}
		if nodes[i].ID != nil {
			nodes[i].Ref = core.Ref(endpoint, "mks", "node", *nodes[i].ID)
			if nodes[i].Ref != nil {
				nodes[i].Ref.Parent = &parent
			}
		}
	}
	return nil
}
func (c *Nodegroups) Get(ctx context.Context, ref types.ResourceRef) (types.Result[DetailedNodegroup], error) {
	validation := validateRef(ref, "nodegroup", c.scope)
	decode := decodeObject("nodegroup", "nodegroup", ref.ID, ref.Parent, func(v *DetailedNodegroup, r types.ResourceRef) { v.Ref = &r }, func(v DetailedNodegroup) error {
		if ref.Parent == nil || *v.ClusterID != ref.Parent.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return attachNodes(v.Nodes, ref, types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"})
	})
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups.get", "GET", resourcePath(ref), 200, &ref, nil, nil, validation, decode)
}
func (c *Nodegroups) Create(ctx context.Context, cluster types.ResourceRef, specs []NodegroupCreateSpec) (types.Result[struct{}], error) {
	validation := validateRef(cluster, "cluster", c.scope)
	if validation == nil && specs == nil {
		validation = types.NewError(types.InvalidArgument)
	}
	for _, spec := range specs {
		if validation != nil {
			break
		}
		validation = inputError(spec)
		if validation == nil {
			validation = validateNodegroupInputs(spec.DedicatedNodegroupConfig, spec.Taints)
		}
		if validation == nil && spec.UserData != nil {
			if _, e := base64.StdEncoding.DecodeString(string(spec.UserData.Reveal())); e != nil {
				validation = types.NewError(types.InvalidArgument)
			}
		}
	}
	body, err := requestBody("nodegroups", specs)
	if validation == nil {
		validation = err
	}
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups.create", "POST", resourcePath(cluster)+"/nodegroups", 204, &cluster, body, nil, validation, empty)
}
func (c *Nodegroups) Replace(ctx context.Context, ref types.ResourceRef, spec NodegroupUpdateSpec) (types.Result[struct{}], error) {
	validation := validateRef(ref, "nodegroup", c.scope)
	if validation == nil {
		validation = inputError(spec)
	}
	if validation == nil {
		validation = validateNodegroupInputs(nil, spec.Taints)
	}
	body, err := requestBody("nodegroup", spec)
	if validation == nil {
		validation = err
	}
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups.replace", "PUT", resourcePath(ref), 204, &ref, body, nil, validation, empty)
}

func validateNodegroupInputs(dedicated *DedicatedNodegroupConfig, taints []NodegroupTaint) error {
	if dedicated != nil && dedicated.Currency != nil && *dedicated.Currency != "main" && *dedicated.Currency != "bonus" {
		return types.NewError(types.InvalidArgument)
	}
	for _, taint := range taints {
		if taint.Effect != nil && *taint.Effect != "NoSchedule" && *taint.Effect != "PreferNoSchedule" && *taint.Effect != "NoExecute" {
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
func (c *Nodegroups) Delete(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups.delete", "DELETE", resourcePath(ref), 204, &ref, nil, nil, validateRef(ref, "nodegroup", c.scope), empty)
}
func (c *Nodegroups) Resize(ctx context.Context, ref types.ResourceRef, spec NodegroupResizeSpec) (types.Result[struct{}], error) {
	validation := validateRef(ref, "nodegroup", c.scope)
	if validation == nil {
		validation = inputError(spec)
	}
	body, err := requestBody("nodegroup", spec)
	if validation == nil {
		validation = err
	}
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups_resize.create", "POST", resourcePath(ref)+"/resize", 204, &ref, body, nil, validation, empty)
}

var nodegroupPending = map[string]bool{"PENDING_CREATE": true, "PENDING_UPDATE": true, "PENDING_DELETE": true, "PENDING_SCALE_UP": true, "PENDING_SCALE_DOWN": true, "PENDING_NODE_REINSTALL": true, "PENDING_NODES_UNSHELVE": true}

func (c *Nodegroups) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := validateRef(ref, "nodegroup", c.scope); e != nil {
		return nil, e
	}
	ref = ownedRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.Get(ctx, ref)
		if e != nil {
			return operations.Observation{}, e
		}
		return observe(ref, *v.Data.Status, v.Metadata, nodegroupPending), nil
	}
	return handle(c.service, ref, receipt, read, map[string]operations.Goal{"nodegroup.active": {Success: map[string]bool{"ACTIVE": true}, Failure: map[string]bool{"ERROR": true}, Pending: nodegroupPending}, "nodegroup.deleted": {Failure: map[string]bool{"ERROR": true}, Pending: deletedPending(nodegroupPending), AbsentSuccess: true}})
}
func (c *Nodes) Get(ctx context.Context, ref types.ResourceRef) (types.Result[Node], error) {
	validation := validateRef(ref, "node", c.scope)
	decode := decodeObject("node", "node", ref.ID, ref.Parent, func(v *Node, r types.ResourceRef) { v.Ref = &r }, func(v Node) error {
		if ref.Parent == nil || *v.NodegroupID != ref.Parent.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	})
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups_nodes.get", "GET", resourcePath(ref), 200, &ref, nil, nil, validation, decode)
}
func (c *Nodes) Delete(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups_nodes.delete", "DELETE", resourcePath(ref), 204, &ref, nil, nil, validateRef(ref, "node", c.scope), empty)
}
func (c *Nodes) Reinstall(ctx context.Context, ref types.ResourceRef) (types.Result[struct{}], error) {
	return invoke(c.service, ctx, "mks.v2.clusters_nodegroups_nodes_reinstall.create", "POST", resourcePath(ref)+"/reinstall", 204, &ref, nil, nil, validateRef(ref, "node", c.scope), empty)
}

func (c *Nodes) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if err := validateRef(ref, "node", c.scope); err != nil {
		return nil, err
	}
	ref = ownedRef(ref)
	read := func(ctx context.Context) (operations.Observation, error) {
		v, err := c.Get(ctx, ref)
		if err != nil {
			return operations.Observation{}, err
		}
		return operations.Observation{Ref: ref, Classification: "pending", Metadata: v.Metadata, ObservedAt: v.Metadata.ObservedAt}, nil
	}
	return handle(c.service, ref, receipt, read, map[string]operations.Goal{"node.deleted": {Pending: map[string]bool{"": true}, AbsentSuccess: true}})
}
