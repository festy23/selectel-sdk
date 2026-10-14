package backups

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type RestoresClient struct{ client *Client }

// Create starts the documented restore action. Accepted/in_progress is not
// application readiness; the source publishes no restore GET/wait operation.
func (c *RestoresClient) Create(ctx context.Context, item types.ResourceRef, name types.ProtectedValue, options types.CallOptions) (types.Result[Restore], error) {
	cl := c.client
	op, bindingErr := cl.runtime.BindDocumented(cl.operation("restores", "create", "POST"), cl.scope)
	op.Success = []int{201}
	body, _ := json.Marshal(map[string]string{"checkpoint_item_id": item.ID, "restore_name": string(name.Reveal())})
	validate := func() error {
		if bindingErr != nil {
			return bindingErr
		}
		if item.Kind != "checkpoint_item" || item.Parent == nil || item.Parent.Kind != "checkpoint" {
			return types.NewError(types.InvalidArgument)
		}
		if e := cl.validateRef(*item.Parent, "checkpoint"); e != nil {
			return e
		}
		p := item.Parent
		if p.Service != item.Service || p.APIGeneration != item.APIGeneration || p.EndpointID != item.EndpointID || p.EndpointURL != item.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		if !utf8.Valid(name.Reveal()) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}
	decode := func(raw []byte, endpoint types.EndpointBinding) (out core.Decoded[Restore], err error) {
		var identity struct {
			ID           *string `json:"id"`
			ProjectID    *string `json:"project_id"`
			CheckpointID *string `json:"checkpoint_id"`
			ItemID       *string `json:"checkpoint_item_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.ID == nil || *identity.ID == "" || identity.ProjectID == nil || identity.CheckpointID == nil || identity.ItemID == nil {
			return out, types.NewError(types.ProtocolError)
		}
		if *identity.ProjectID != cl.scope.ProjectID || *identity.CheckpointID != item.Parent.ID || *identity.ItemID != item.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(endpoint, "backups", "restore", *identity.ID)
		ref.Parent = &item
		out.Refs = []types.ResourceRef{*ref}
		var value Restore
		if json.Unmarshal(raw, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if e := value.Validate(); e != nil {
			return out, e
		}
		value.Ref = ref
		out.Data = value
		return out, nil
	}
	return core.DoJSON(cl.runtime, ctx, op, core.Request{Scope: cl.scope, Path: cl.path("restores/"), Body: body, Refs: []types.ResourceRef{item}, PinnedEndpoint: pin(item), Options: options, Validate: validate}, decode)
}
