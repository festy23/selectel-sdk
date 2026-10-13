package mks

import (
	"context"
	"encoding/json"
	"math/big"
	"net/url"
	"strconv"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/operations"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

type Tasks struct{ service }
type TaskOptions struct {
	WithErrorDetails *bool `json:"with_error_details,omitempty"`
}
type TaskListOptions struct {
	WithErrorDetails *bool          `json:"with_error_details,omitempty"`
	Offset           *types.Integer `json:"offset,omitempty"`
}

func taskQuery(options TaskOptions) url.Values {
	q := url.Values{}
	if options.WithErrorDetails != nil {
		q.Set("with_error_details", strconv.FormatBool(*options.WithErrorDetails))
	}
	return q
}
func (c *Tasks) Get(ctx context.Context, ref types.ResourceRef, options TaskOptions) (types.Result[Task], error) {
	validation := validateRef(ref, "task", c.scope)
	decode := decodeObject("task", "task", ref.ID, ref.Parent, func(v *Task, r types.ResourceRef) { v.Ref = &r }, func(v Task) error {
		if ref.Parent == nil || *v.ClusterID != ref.Parent.ID {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	})
	return invoke(c.service, ctx, "mks.v2.clusters_tasks.get", "GET", resourcePath(ref), 200, &ref, nil, taskQuery(options), validation, decode)
}
func (c *Tasks) Iterate(cluster types.ResourceRef, filters TaskListOptions, options pagination.Options) (*pagination.Iterator[Task], error) {
	if e := validateRef(cluster, "cluster", c.scope); e != nil {
		return nil, e
	}
	initial := "0"
	if filters.Offset != nil {
		v, ok := new(big.Int).SetString(filters.Offset.String(), 10)
		if !ok || v.Sign() < 0 || v.BitLen() > 64 {
			return nil, types.NewError(types.InvalidArgument)
		}
		initial = v.String()
	}
	op, err := c.runtime.BindDocumented(core.Operation{ID: "mks.v2.clusters_tasks.list", Service: "mks", APIGeneration: "v2", Method: "GET", Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}}, c.scope)
	if err != nil {
		return nil, err
	}
	semantic := struct {
		Cluster types.ResourceRef `json:"cluster"`
		Filters TaskListOptions   `json:"filters"`
	}{cluster, filters}
	plan, err := c.runtime.PagePlan(op.ID, "v2", c.scope, op.EndpointID, semantic)
	if err != nil {
		return nil, err
	}
	query := taskQuery(TaskOptions{WithErrorDetails: filters.WithErrorDetails})
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[Task], error) {
		if cursor == "" {
			cursor = initial
		}
		offset, ok := new(big.Int).SetString(cursor, 10)
		if !ok || offset.Sign() < 0 || offset.BitLen() > 64 {
			return pagination.SourcePage[Task]{}, types.NewError(types.InvalidArgument)
		}
		q := url.Values{"offset": {offset.String()}, "limit": {strconv.FormatUint(uint64(size), 10)}}
		for k, v := range query {
			q[k] = append([]string(nil), v...)
		}
		done := false
		next := ""
		provider := pagination.ProviderMetadata{}
		decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[[]Task], error) {
			var body struct {
				Count *types.Integer `json:"count"`
				Tasks []Task         `json:"tasks"`
			}
			if json.Unmarshal(raw, &body) != nil || body.Count == nil || body.Tasks == nil {
				return core.Decoded[[]Task]{}, types.NewError(types.ProtocolError)
			}
			total, ok := new(big.Int).SetString(body.Count.String(), 10)
			if !ok || total.Sign() < 0 {
				return core.Decoded[[]Task]{}, types.NewError(types.ProtocolError)
			}
			for i := range body.Tasks {
				v := &body.Tasks[i]
				if e := v.Validate(); e != nil {
					return core.Decoded[[]Task]{}, e
				}
				if *v.ClusterID != cluster.ID || *v.ID == "" {
					return core.Decoded[[]Task]{}, types.NewError(types.ScopeMismatch)
				}
				v.Ref = core.Ref(endpoint, "mks", "task", *v.ID)
				v.Ref.Parent = &cluster
			}
			advance := new(big.Int).Add(offset, big.NewInt(int64(len(body.Tasks))))
			done = len(body.Tasks) == 0 || advance.Cmp(total) >= 0
			provider.TotalCount = body.Count
			if !done {
				next = advance.String()
			}
			return core.Decoded[[]Task]{Data: body.Tasks}, nil
		}
		v, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: resourcePath(cluster) + "/tasks", Query: q, Refs: []types.ResourceRef{cluster}, PinnedEndpoint: pinned(cluster)}, decode)
		if e != nil {
			provider = pagination.ProviderMetadata{}
		}
		return pagination.SourcePage[Task]{Items: v.Data, Next: next, Done: done, Metadata: v.Metadata, Provider: provider}, e
	}
	return pagination.New(plan, options, fetch)
}
func (c *Tasks) ListPage(ctx context.Context, cluster types.ResourceRef, f TaskListOptions, o pagination.Options) (pagination.Page[Task], error) {
	it, e := c.Iterate(cluster, f, o)
	if e != nil {
		return pagination.Page[Task]{}, e
	}
	return it.Next(ctx)
}
func (c *Tasks) Collect(ctx context.Context, cluster types.ResourceRef, f TaskListOptions, o pagination.Options) (pagination.Collection[Task], error) {
	it, e := c.Iterate(cluster, f, o)
	if e != nil {
		return pagination.Collection[Task]{Items: []Task{}, StopReason: "error"}, e
	}
	return it.Collect(ctx)
}
func (c *Tasks) Handle(ref types.ResourceRef, receipt *types.Receipt) (*operations.Handle, error) {
	if e := validateRef(ref, "task", c.scope); e != nil {
		return nil, e
	}
	ref = ownedRef(ref)
	pending := map[string]bool{"IN_PROGRESS": true, "IN_QUEUE": true}
	read := func(ctx context.Context) (operations.Observation, error) {
		v, e := c.Get(ctx, ref, TaskOptions{})
		if e != nil {
			return operations.Observation{}, e
		}
		return observe(ref, *v.Data.Status, v.Metadata, pending), nil
	}
	return handle(c.service, ref, receipt, read, map[string]operations.Goal{"task.done": {Success: map[string]bool{"DONE": true}, Failure: map[string]bool{"ERROR": true, "CANCELED": true}, Pending: pending}})
}
