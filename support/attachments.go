package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
)

type ReadBounds struct{ MaxItems uint }

func limitRows[T any](rows *[]T, limit uint) bool {
	if rows == nil || uint(len(*rows)) <= limit {
		return false
	}
	*rows = (*rows)[:limit]
	return true
}
func (c *AttachmentsClient) readLimit(op core.Operation, bounds ReadBounds) uint {
	if bounds.MaxItems > 0 {
		return bounds.MaxItems
	}
	plan, e := c.runtime.PagePlan(op.ID, "attachments_v1", c.scope, op.EndpointID, nil)
	if e != nil {
		return 1
	}
	return plan.Defaults.MaxItems
}
func (c *AttachmentsClient) GetMetadataBatch(ctx context.Context, refs []types.ResourceRef, bounds ReadBounds, o types.CallOptions) (types.Result[[]AttachmentUploadedFile], error) {
	op, bindErr := c.operation("tickets.attachments_v1.files.get_batch", "GET")
	query := url.Values{}
	known := map[string]types.ResourceRef{}
	var refErr error
	for _, ref := range refs {
		if e := c.refError(ref); e != nil {
			refErr = e
		}
		if _, exists := known[ref.ID]; exists {
			refErr = types.NewError(types.InvalidArgument)
		}
		known[ref.ID] = ref
		query.Add("file_uuid", ref.ID)
	}
	limit := c.readLimit(op, bounds)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/public/files/info", Query: query, Options: o, Refs: refs, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if len(refs) == 0 {
			return types.NewError(types.InvalidArgument)
		}
		return refErr
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]AttachmentUploadedFile], error) {
		out := core.Decoded[[]AttachmentUploadedFile]{}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data == nil {
			return out, types.NewError(types.ProtocolError)
		}
		seen := map[string]bool{}
		for i := range out.Data {
			row := &out.Data[i]
			if row.FileUUID == nil {
				return out, types.NewError(types.ProtocolError)
			}
			ref, ok := known[*row.FileUUID]
			if !ok {
				return out, types.NewError(types.ScopeMismatch)
			}
			if seen[ref.ID] {
				return out, types.NewError(types.ProtocolError)
			}
			seen[ref.ID] = true
			row.Ref = &ref
			if e := row.Validate(); e != nil {
				return out, e
			}
		}
		if limitRows(&out.Data, limit) {
			out.PartialRead = true
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	})
}
func (c *AttachmentsClient) formatList(ctx context.Context, id, path string, bounds ReadBounds, o types.CallOptions) (types.Result[AttachmentExtensionsOrMimetypesItems], error) {
	op, bindErr := c.operation(id, "GET")
	op.Rejected = []int{401}
	limit := c.readLimit(op, bounds)
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: path, Options: o, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[AttachmentExtensionsOrMimetypesItems], error) {
		out := core.Decoded[AttachmentExtensionsOrMimetypesItems]{}
		if json.Unmarshal(raw, &out.Data) != nil || out.Data.Items == nil {
			return out, types.NewError(types.ProtocolError)
		}
		if limitRows(out.Data.Items, limit) {
			out.PartialRead = true
			return out, types.NewError(types.ResponseLimit)
		}
		return out, nil
	})
}
func (c *AttachmentsClient) Extensions(ctx context.Context, bounds ReadBounds, o types.CallOptions) (types.Result[AttachmentExtensionsOrMimetypesItems], error) {
	return c.formatList(ctx, "tickets.attachments_v1.extensions.list", "/public/files/extensions", bounds, o)
}
func (c *AttachmentsClient) MIMETypes(ctx context.Context, bounds ReadBounds, o types.CallOptions) (types.Result[AttachmentExtensionsOrMimetypesItems], error) {
	return c.formatList(ctx, "tickets.attachments_v1.mimetypes.list", "/public/files/mimetypes", bounds, o)
}
