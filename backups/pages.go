package backups

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type PlansQuery struct {
	Limit                    *types.Integer
	Marker, Name, VolumeName *string
}
type CheckpointsQuery struct {
	Limit                        *types.Integer
	Marker, PlanName, VolumeName *string
}

func pageQuery(limit *types.Integer, fields map[string]*string) (url.Values, error) {
	q := url.Values{}
	if limit != nil {
		n, e := types.ParseInteger(limit.String())
		if e != nil || n.String() == "0" || n.String()[0] == '-' {
			return nil, types.NewError(types.InvalidArgument)
		}
		q.Set("limit", n.String())
	}
	for name, v := range fields {
		if v != nil {
			q.Set(name, *v)
		}
	}
	return q, nil
}

// ListPage preserves provider total and one bounded page. No next/end marker is
// manufactured from a short page or a total that can change between calls.
func (c *PlansClient) ListPage(ctx context.Context, filters PlansQuery, options types.CallOptions) (types.Result[PlansPage], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("plans", "list", "GET"), cl.scope)
	q, queryErr := pageQuery(filters.Limit, map[string]*string{"marker": filters.Marker, "name": filters.Name, "volume_name": filters.VolumeName})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return queryErr
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[PlansPage], error) {
		var value PlansPage
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[PlansPage]{}, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[PlansPage]{}, e
		}
		for i := range value.Plans {
			if value.Plans[i].ID != nil {
				value.Plans[i].Ref = core.Ref(endpoint, "backups", "plan", *value.Plans[i].ID)
			}
		}
		return core.Decoded[PlansPage]{Data: value}, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("plans/"), Query: q, Options: options, Validate: validate}, decode)
}

func (c *CheckpointsClient) ListPage(ctx context.Context, filters CheckpointsQuery, options types.CallOptions) (types.Result[CheckpointsPage], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("checkpoints", "list", "GET"), cl.scope)
	q, queryErr := pageQuery(filters.Limit, map[string]*string{"marker": filters.Marker, "plan_name": filters.PlanName, "volume_name": filters.VolumeName})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return queryErr
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[CheckpointsPage], error) {
		var value CheckpointsPage
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[CheckpointsPage]{}, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return core.Decoded[CheckpointsPage]{}, e
		}
		for i := range value.Checkpoints {
			if e := bindCheckpoint(&value.Checkpoints[i], endpoint); e != nil {
				return core.Decoded[CheckpointsPage]{}, e
			}
		}
		return core.Decoded[CheckpointsPage]{Data: value}, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("checkpoints/"), Query: q, Options: options, Validate: validate}, decode)
}
