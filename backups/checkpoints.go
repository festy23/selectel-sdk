package backups

import (
	"context"
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type CheckpointsClient struct{ client *Client }

func (c *Client) validateRef(ref types.ResourceRef, kind string) error {
	if ref.Kind != kind {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Parent != nil {
		p := ref.Parent
		if p.Service != ref.Service || p.APIGeneration != ref.APIGeneration || p.EndpointID != ref.EndpointID || p.EndpointURL != ref.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		if kind != "checkpoint" || p.Kind != "plan" || p.Parent != nil {
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
func pin(ref types.ResourceRef) *types.EndpointBinding {
	return &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, APIGeneration: ref.APIGeneration, Scope: ref.Scope, Audience: "selectel"}
}
func bindCheckpoint(value *Checkpoint, endpoint types.EndpointBinding) error {
	if e := value.Validate(); e != nil {
		return e
	}
	ref := core.Ref(endpoint, "backups", "checkpoint", *value.ID)
	if ref == nil || value.PlanID == nil || *value.PlanID == "" {
		return types.NewError(types.ProtocolError)
	}
	ref.Parent = core.Ref(endpoint, "backups", "plan", *value.PlanID)
	value.Ref = ref
	for i := range value.CheckpointItems {
		item := &value.CheckpointItems[i]
		if item.CheckpointID == nil || *item.CheckpointID != *value.ID {
			return types.NewError(types.ScopeMismatch)
		}
		item.Ref = core.Ref(endpoint, "backups", "checkpoint_item", *item.ID)
		if item.Ref == nil {
			return types.NewError(types.ProtocolError)
		}
		item.Ref.Parent = ref
	}
	return nil
}
func (c *CheckpointsClient) Get(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[Checkpoint], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("checkpoints", "get", "GET"), cl.scope)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return cl.validateRef(ref, "checkpoint")
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Checkpoint], error) {
		var value Checkpoint
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[Checkpoint]{}, types.NewError(types.ProtocolError)
		}
		if e := bindCheckpoint(&value, endpoint); e != nil {
			return core.Decoded[Checkpoint]{}, e
		}
		if *value.ID != ref.ID || (ref.Parent != nil && *value.PlanID != ref.Parent.ID) {
			return core.Decoded[Checkpoint]{}, types.NewError(types.ScopeMismatch)
		}
		return core.Decoded[Checkpoint]{Data: value}, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("checkpoints/") + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: validate}, decode)
}
func (c *CheckpointsClient) Delete(ctx context.Context, ref types.ResourceRef, options types.CallOptions) (types.Result[DeletedCheckpoint], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("checkpoints", "delete", "DELETE"), cl.scope)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return cl.validateRef(ref, "checkpoint")
	}
	decode := func(raw []byte, _ types.EndpointBinding) (core.Decoded[DeletedCheckpoint], error) {
		var v DeletedCheckpoint
		if json.Unmarshal(raw, &v) != nil {
			return core.Decoded[DeletedCheckpoint]{}, types.NewError(types.ProtocolError)
		}
		if e := v.Validate(); e != nil {
			return core.Decoded[DeletedCheckpoint]{}, e
		}
		if *v.ID != ref.ID {
			return core.Decoded[DeletedCheckpoint]{}, types.NewError(types.ScopeMismatch)
		}
		v.Ref = &ref
		return core.Decoded[DeletedCheckpoint]{Data: v}, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("checkpoints/") + segment(ref.ID), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: validate}, decode)
}
func (c *CheckpointsClient) DeleteChain(ctx context.Context, chain types.ResourceRef, options types.CallOptions) (types.Result[DeletedChain], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("checkpoint_chains", "delete", "DELETE"), cl.scope)
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		return cl.validateRef(chain, "checkpoint_chain")
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[DeletedChain], err error) {
		var env struct {
			Deleted []json.RawMessage `json:"deleted"`
		}
		if json.Unmarshal(raw, &env) != nil || env.Deleted == nil {
			return out, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for _, row := range env.Deleted {
			var id struct {
				ID     *string `json:"id"`
				PlanID *string `json:"plan_id"`
			}
			if json.Unmarshal(row, &id) != nil || id.ID == nil || *id.ID == "" {
				if err == nil {
					err = types.NewError(types.ProtocolError)
				}
				continue
			}
			if seen[*id.ID] {
				if err == nil {
					err = types.NewError(types.ProtocolError)
				}
				continue
			}
			seen[*id.ID] = true
			ref := core.Ref(endpoint, "backups", "checkpoint", *id.ID)
			if id.PlanID != nil && *id.PlanID != "" {
				ref.Parent = core.Ref(endpoint, "backups", "plan", *id.PlanID)
			}
			out.Refs = append(out.Refs, *ref)
		}
		if err != nil {
			return out, err
		}
		var value DeletedChain
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return out, e
		}
		for i := range value.Deleted {
			if e := bindCheckpoint(&value.Deleted[i], endpoint); e != nil {
				return out, e
			}
		}
		out.Data = value
		return out, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("checkpoints/"), Query: url.Values{"chain_id": {chain.ID}}, Refs: []types.ResourceRef{chain}, PinnedEndpoint: pin(chain), Options: options, Validate: validate}, decode)
}
