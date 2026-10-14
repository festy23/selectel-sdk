package cdn

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func opaqueID(id string) bool {
	return id != "" && len(id) <= 1024 && utf8.ValidString(id) && !strings.ContainsAny(id, "/\r\n\x00") && id != "." && id != ".."
}
func (c *Client) CreateCacheTask(ctx context.Context, spec Task, o types.CallOptions) (types.Result[CreateTaskResponse], error) {
	op, bindErr := c.operation("cdn.v3.cache_tasks.create", "POST")
	body, encodeErr := wireBody(spec)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cache/tasks", Body: body, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if encodeErr != nil {
			return encodeErr
		}
		if spec.Action == nil || (*spec.Action != "delete" && *spec.Action != "preload") {
			return types.NewError(types.InvalidArgument)
		}
		if spec.ActionType != nil {
			kind := *spec.ActionType
			if kind != "single" && kind != "full" && kind != "playlist" {
				return types.NewError(types.InvalidArgument)
			}
			if (kind == "full" && *spec.Action != "delete") || (kind == "playlist" && *spec.Action != "preload") {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[CreateTaskResponse], error) {
		var identity struct {
			TaskID *string `json:"task_id"`
		}
		if json.Unmarshal(raw, &identity) != nil || identity.TaskID == nil || !opaqueID(*identity.TaskID) {
			return core.Decoded[CreateTaskResponse]{}, types.NewError(types.ProtocolError)
		}
		ref := core.Ref(ep, "cdn", "cache_task", *identity.TaskID)
		out := core.Decoded[CreateTaskResponse]{Refs: []types.ResourceRef{*ref}}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		out.Data.Ref = ref
		return out, out.Data.Validate()
	})
}
func (c *Client) GetCacheTask(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[TaskResult], error) {
	op, bindErr := c.operation("cdn.v3.cache_tasks.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/cache/tasks/" + url.PathEscape(ref.ID), PinnedEndpoint: pin(ref), Refs: []types.ResourceRef{ref}, Options: o, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref, "cache_task")
	}}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[TaskResult], error) {
		var out TaskResult
		if json.Unmarshal(raw, &out) != nil {
			return core.Decoded[TaskResult]{}, types.NewError(types.ProtocolError)
		}
		if out.TaskID == nil || *out.TaskID != ref.ID {
			return core.Decoded[TaskResult]{}, types.NewError(types.ScopeMismatch)
		}
		known := ref
		out.Ref = &known
		return core.Decoded[TaskResult]{Data: out}, out.Validate()
	})
}
