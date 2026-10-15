package mkslegacy

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
)

type TaskOptions struct {
	WithErrorDetails *bool `json:"with_error_details,omitempty"`
	WithTaskDetails  *bool `json:"with_task_details,omitempty"`
}

func taskQuery(options TaskOptions) url.Values {
	q := url.Values{}
	if options.WithErrorDetails != nil {
		q.Set("with_error_details", strconv.FormatBool(*options.WithErrorDetails))
	}
	if options.WithTaskDetails != nil {
		q.Set("with_task_details", strconv.FormatBool(*options.WithTaskDetails))
	}
	return q
}
func (c *Client) GetTask(ctx context.Context, ref types.ResourceRef, options TaskOptions, call types.CallOptions) (types.Result[Task], error) {
	op := core.Operation{ID: "mks.v1.clusters_tasks.get", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(ref), Query: taskQuery(options), Refs: []types.ResourceRef{ref}, Options: call, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.validateNested(ref, "task", op)
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[Task], error) {
		d := core.Decoded[Task]{}
		var env struct {
			Task json.RawMessage `json:"task"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Task, &d.Data) != nil || d.Data.Validate() != nil || d.Data.ID == nil || d.Data.ClusterID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *d.Data.ID != ref.ID || *d.Data.ClusterID != ref.Parent.ID {
			return d, types.NewError(types.ScopeMismatch)
		}
		owned := cloneRef(ref)
		d.Data.Ref = &owned
		return d, nil
	})
}

type TaskListOptions struct {
	WithErrorDetails *bool          `json:"with_error_details,omitempty"`
	Offset           *types.Integer `json:"Offset,omitempty"`
	Limit            *types.Integer `json:"Limit,omitempty"`
}

// ListTasksPage preserves one source page. It never treats count or a short/empty
// array as an undocumented end rule, and never synthesizes a traversal cursor.
func (c *Client) ListTasksPage(ctx context.Context, cluster types.ResourceRef, options TaskListOptions, call types.CallOptions) (pagination.Page[Task], error) {
	op := core.Operation{ID: "mks.v1.clusters_tasks.list", Service: "mks", APIGeneration: "v1", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	q := taskQuery(TaskOptions{WithErrorDetails: options.WithErrorDetails})
	if options.Offset != nil {
		q.Set("Offset", options.Offset.String())
	}
	if options.Limit != nil {
		q.Set("Limit", options.Limit.String())
	}
	type response struct {
		Count *types.Integer `json:"count"`
		Tasks []Task         `json:"tasks"`
	}
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(cluster) + "/tasks", Query: q, Refs: []types.ResourceRef{cluster}, Options: call, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRoot(cluster, "cluster", op); e != nil {
			return e
		}
		for _, selector := range []struct {
			v        *types.Integer
			positive bool
		}{{options.Offset, false}, {options.Limit, true}} {
			if selector.v == nil {
				continue
			}
			n, ok := new(big.Int).SetString(selector.v.String(), 10)
			if !ok || n.Sign() < 0 || n.BitLen() > 63 || selector.positive && n.Sign() == 0 {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[response], error) {
		d := core.Decoded[response]{}
		if json.Unmarshal(raw, &d.Data) != nil || d.Data.Tasks == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if d.Data.Count != nil {
			n, ok := new(big.Int).SetString(d.Data.Count.String(), 10)
			if !ok || n.Sign() < 0 {
				return d, types.NewError(types.ProtocolError)
			}
		}
		for i := range d.Data.Tasks {
			v := &d.Data.Tasks[i]
			if v.Validate() != nil || v.ID == nil || *v.ID == "" || v.ClusterID == nil {
				return d, types.NewError(types.ProtocolError)
			}
			if *v.ClusterID != cluster.ID {
				return d, types.NewError(types.ScopeMismatch)
			}
			ref := core.Ref(ep, "mks", "task", *v.ID)
			parent := cloneRef(cluster)
			ref.Parent = &parent
			v.Ref = ref
		}
		return d, nil
	})
	page := pagination.Page[Task]{Items: out.Data.Tasks, Metadata: out.Metadata, Provider: pagination.ProviderMetadata{ReportedCount: out.Data.Count}}
	if e != nil {
		return page, e
	}
	return page, types.NewError(types.Incomplete)
}
