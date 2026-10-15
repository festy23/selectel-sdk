package filestorage

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"unicode/utf8"
)

func (c *Client) AllowAccess(ctx context.Context, share types.ResourceRef, input AccessCreateRequest, options types.CallOptions) (types.Result[AccessRule], error) {
	op := core.Operation{ID: "filestorage.v2_73.shares.allow", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200, 202}, Rejected: []int{400, 401, 403, 404}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(struct {
		Input AccessCreateRequest `json:"allow_access"`
	}{input})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/shares/" + url.PathEscape(share.ID) + "/action", Body: body, Refs: []types.ResourceRef{share}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateShare(share, op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil || input.AccessType == nil || *input.AccessType == "" || !utf8.ValidString(*input.AccessType) || input.AccessTo == nil || input.AccessTo.Empty() {
			return types.NewError(types.InvalidArgument)
		}
		if input.AccessLevel != nil && *input.AccessLevel != "ro" && *input.AccessLevel != "rw" {
			return types.NewError(types.InvalidArgument)
		}
		if input.Metadata != nil {
			var m map[string]string
			if json.Unmarshal(input.Metadata.Reveal(), &m) != nil || m == nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[AccessRule], error) {
		d := core.Decoded[AccessRule]{}
		var env struct {
			Access json.RawMessage `json:"access"`
		}
		var identity struct {
			ID      *string `json:"id"`
			ShareID *string `json:"share_id"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Access, &identity) != nil || identity.ID == nil || *identity.ID == "" || identity.ShareID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *identity.ShareID != share.ID {
			return d, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "filestorage", "access_rule", *identity.ID)
		parent := cloneRef(share)
		ref.Parent = &parent
		d.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(env.Access, &d.Data) != nil || d.Data.Validate() != nil {
			return d, types.NewError(types.ProtocolError)
		}
		d.Data.Ref = ref
		return d, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// DenyAccess requires the caller's exact rule and share parent; an ID from a
// response association or another generation is never converted or adopted.
func (c *Client) DenyAccess(ctx context.Context, rule types.ResourceRef, options types.CallOptions) (types.Result[struct{}], error) {
	op := core.Operation{ID: "filestorage.v2_73.shares.deny", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{202}, NoContentSuccess: []int{202}, Rejected: []int{400, 401, 403, 404}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	parent := types.ResourceRef{}
	if rule.Parent != nil {
		parent = cloneRef(*rule.Parent)
	}
	body, _ := json.Marshal(map[string]any{"deny_access": map[string]string{"access_id": rule.ID}})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/shares/" + url.PathEscape(parent.ID) + "/action", Body: body, Refs: []types.ResourceRef{rule}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if rule.Parent == nil || rule.Validate() != nil {
			return types.NewError(types.InvalidArgument)
		}
		if e := c.validateShare(parent, op); e != nil {
			return e
		}
		if rule.Kind != "access_rule" || rule.Service != parent.Service || rule.APIGeneration != parent.APIGeneration || rule.Scope != parent.Scope || rule.EndpointID != parent.EndpointID || rule.EndpointURL != parent.EndpointURL {
			return types.NewError(types.ScopeMismatch)
		}
		return nil
	}}, emptyResult)
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
func emptyResult(raw []byte, _ types.EndpointBinding) (core.Decoded[struct{}], error) {
	if len(bytes.TrimSpace(raw)) != 0 {
		return core.Decoded[struct{}]{}, types.NewError(types.ProtocolError)
	}
	return core.Decoded[struct{}]{}, nil
}
