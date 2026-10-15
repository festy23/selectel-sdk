package filestorage

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strings"
)

func (c *Client) Extend(ctx context.Context, share types.ResourceRef, input ExtendRequest, options types.CallOptions) (types.Result[struct{}], error) {
	op := core.Operation{ID: "filestorage.v2_73.shares.extend", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{202}, NoContentSuccess: []int{202}, Rejected: []int{400, 401, 403, 404}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(struct {
		Extend ExtendRequest `json:"extend"`
	}{input})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/shares/" + url.PathEscape(share.ID) + "/action", Body: body, Refs: []types.ResourceRef{share}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateShare(share, op); e != nil {
			return e
		}
		if bodyErr != nil || input.Validate() != nil || input.NewSize == nil || input.NewSize.String() == "" || input.NewSize.String() == "0" || strings.HasPrefix(input.NewSize.String(), "-") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, emptyResult)
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}

// ExtendLegacy sends the first-party os-extend example without negotiation.
// legacy_os_extend identifies that wire variant, not upstream v2.0 support.
func (c *Client) ExtendLegacy(ctx context.Context, share types.ResourceRef, newSize types.Integer, options types.CallOptions) (types.Result[struct{}], error) {
	op := core.Operation{ID: "filestorage.legacy_os_extend.shares.extend", Service: "filestorage", APIGeneration: "legacy_os_extend", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{202}, NoContentSuccess: []int{202}, Rejected: []int{400, 401, 403, 404}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	body, bodyErr := wireBody(map[string]any{"os-extend": map[string]any{"new_size": newSize}})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/shares/" + url.PathEscape(share.ID) + "/action", Body: body, Refs: []types.ResourceRef{share}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateShare(share, op); e != nil {
			return e
		}
		if bodyErr != nil || newSize.String() == "" || newSize.String() == "0" || strings.HasPrefix(newSize.String(), "-") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, emptyResult)
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
