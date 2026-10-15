package mkslegacy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"strings"
)

func (c *Client) ListNodegroups(ctx context.Context, cluster types.ResourceRef, options pagination.Options) (pagination.Collection[Nodegroup], error) {
	op := core.Operation{ID: "mks.v1.clusters_nodegroups.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[Nodegroup]{}, e
	}
	if e = c.validateRoot(cluster, "cluster", op); e != nil {
		return pagination.Collection[Nodegroup]{}, e
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/clusters/" + url.PathEscape(cluster.ID) + "/nodegroups", Refs: []types.ResourceRef{cluster}}, cluster, options, "nodegroups", func(v *Nodegroup, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" || v.ClusterID == nil {
			return types.NewError(types.ProtocolError)
		}
		if *v.ClusterID != cluster.ID {
			return types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "mks", "nodegroup", *v.ID)
		parent := cloneRef(cluster)
		ref.Parent = &parent
		if e := bindNodes(v.Nodes, *ref, ep); e != nil {
			return e
		}
		v.Ref = ref
		return nil
	})
	if e != nil {
		return pagination.Collection[Nodegroup]{}, e
	}
	return iterator.Collect(ctx)
}
func bindNodes(nodes []Node, group types.ResourceRef, ep types.EndpointBinding) error {
	for i := range nodes {
		v := &nodes[i]
		if v.ID == nil || *v.ID == "" || v.Validate() != nil {
			return types.NewError(types.ProtocolError)
		}
		if v.NodegroupID != nil && *v.NodegroupID != group.ID {
			return types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "mks", "node", *v.ID)
		parent := cloneRef(group)
		ref.Parent = &parent
		v.Ref = ref
	}
	return nil
}

func (c *Client) GetNodegroup(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DetailedNodegroup], error) {
	op := core.Operation{ID: "mks.v1.clusters_nodegroups.get", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(ref), Refs: []types.ResourceRef{ref}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateNested(ref, "nodegroup", op)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[DetailedNodegroup], error) {
		d := core.Decoded[DetailedNodegroup]{}
		var env struct {
			Nodegroup json.RawMessage `json:"nodegroup"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Nodegroup, &d.Data) != nil || d.Data.Validate() != nil || d.Data.ID == nil || d.Data.ClusterID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *d.Data.ID != ref.ID || *d.Data.ClusterID != ref.Parent.ID {
			return d, types.NewError(types.ScopeMismatch)
		}
		if e := bindNodes(d.Data.Nodes, ref, ep); e != nil {
			return d, e
		}
		owned := cloneRef(ref)
		d.Data.Ref = &owned
		return d, nil
	})
}

func (c *Client) CreateNodegroup(ctx context.Context, cluster types.ResourceRef, input NodegroupCreate, options types.CallOptions) (types.Result[struct{}], error) {
	body, bodyErr := requestBody("nodegroup", input)
	return acknowledge(c, ctx, "mks.v1.clusters_nodegroups.create", "POST", resourcePath(cluster)+"/nodegroups", []types.ResourceRef{cluster}, body, options, func(op core.Operation) error {
		if e := c.validateRoot(cluster, "cluster", op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if input.UserData != nil {
			if _, e := base64.StdEncoding.DecodeString(string(input.UserData.Reveal())); e != nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	})
}

func (c *Client) UpdateNodegroup(ctx context.Context, group types.ResourceRef, input NodegroupUpdate, options types.CallOptions) (types.Result[struct{}], error) {
	body, bodyErr := requestBody("nodegroup", input)
	return acknowledge(c, ctx, "mks.v1.clusters_nodegroups.replace", "PUT", resourcePath(group), []types.ResourceRef{group}, body, options, func(op core.Operation) error {
		if e := c.validateNested(group, "nodegroup", op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}

func (c *Client) DeleteNodegroup(ctx context.Context, group types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	return acknowledge(c, ctx, "mks.v1.clusters_nodegroups.delete", "DELETE", resourcePath(group), []types.ResourceRef{group}, nil, options, func(op core.Operation) error { return c.validateNested(group, "nodegroup", op) })
}

func (c *Client) ResizeNodegroup(ctx context.Context, group types.ResourceRef, desired types.Integer, options types.CallOptions) (types.Result[struct{}], error) {
	body, bodyErr := requestBody("nodegroup", map[string]any{"desired": desired})
	return acknowledge(c, ctx, "mks.v1.clusters_nodegroups_resize.create", "POST", resourcePath(group)+"/resize", []types.ResourceRef{group}, body, options, func(op core.Operation) error {
		if e := c.validateNested(group, "nodegroup", op); e != nil {
			return e
		}
		if bodyErr != nil || desired.String() == "" || strings.HasPrefix(desired.String(), "-") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	})
}
