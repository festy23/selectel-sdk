// Package support exposes ticket management and attachment APIs; creating a
// ticket or comment is an explicit mutation, never part of observation.
package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"regexp"
)

type Client struct {
	Attachments *AttachmentsClient
	Tickets     *TicketsClient
	Legacy      *LegacyClient
}
type AttachmentsClient struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, s types.Scope) *Client {
	return &Client{Attachments: &AttachmentsClient{runtime: r, scope: s}, Tickets: &TicketsClient{runtime: r, scope: s}, Legacy: &LegacyClient{runtime: r, scope: s}}
}

var fileUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (c *AttachmentsClient) operation(id, method string) (core.Operation, error) {
	effect := "read"
	if method != "GET" {
		effect = "mutation"
	}
	op := core.Operation{ID: id, Service: "tickets", APIGeneration: "attachments_v1", Method: method, Effect: effect, Auth: types.StaticToken, Success: []int{200}, Rejected: []int{401, 422}}
	if c.scope.ProjectID != "" || c.scope.Region != "" || c.scope.Pool != "" {
		return op, types.NewError(types.ScopeMismatch)
	}
	return c.runtime.BindDocumented(op, c.scope)
}
func (c *AttachmentsClient) File(id string) (types.ResourceRef, error) {
	if !fileUUID.MatchString(id) {
		return types.ResourceRef{}, types.NewError(types.InvalidArgument)
	}
	op, e := c.operation("tickets.attachments_v1.files.get", "GET")
	if e != nil {
		return types.ResourceRef{}, e
	}
	return *core.Ref(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, Scope: c.scope, APIGeneration: "attachments_v1"}, "tickets", "file", id), nil
}
func (c *AttachmentsClient) refError(ref types.ResourceRef) error {
	if ref.Validate() != nil || !fileUUID.MatchString(ref.ID) || ref.Parent != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Service != "tickets" || ref.APIGeneration != "attachments_v1" || ref.Kind != "file" || ref.Scope != c.scope {
		return types.NewError(types.ScopeMismatch)
	}
	op, e := c.operation("tickets.attachments_v1.files.get", "GET")
	if e != nil {
		return e
	}
	if op.EndpointID != ref.EndpointID || op.FixedEndpoint != ref.EndpointURL {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
func (c *AttachmentsClient) GetMetadata(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[AttachmentUploadedFile], error) {
	op, bindErr := c.operation("tickets.attachments_v1.files.get", "GET")
	return core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/public/files/uuid/" + url.PathEscape(ref.ID), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: &types.EndpointBinding{ID: ref.EndpointID, URL: ref.EndpointURL, Scope: ref.Scope, APIGeneration: ref.APIGeneration, Audience: "selectel"}, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		return c.refError(ref)
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[AttachmentUploadedFile], error) {
		out := core.Decoded[AttachmentUploadedFile]{}
		if json.Unmarshal(raw, &out.Data) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if out.Data.FileUUID != nil && *out.Data.FileUUID != ref.ID {
			return out, types.NewError(types.ScopeMismatch)
		}
		out.Data.Ref = core.Ref(ep, "tickets", "file", ref.ID)
		out.Refs = []types.ResourceRef{*out.Data.Ref}
		return out, out.Data.Validate()
	})
}
