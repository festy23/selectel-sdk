package filestorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"strings"
)

// CreateShare supports the documented explicit network/type scenario. It does
// not reserve quota or claim that acceptance makes the export ready to mount.
func (c *Client) CreateShare(ctx context.Context, network, shareType types.ResourceRef, input ShareCreateRequest, options types.CallOptions) (types.Result[Share], error) {
	op := core.Operation{ID: "filestorage.v2_73.shares.create", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodPost, Effect: "mutation", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200, 202}, Rejected: []int{400, 401, 403, 404, 409, 413, 422}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, bindErr := c.runtime.BindDocumented(op, c.scope)
	originalNetwork, originalType := input.ShareNetworkID, input.ShareType
	input.ShareNetworkID = &network.ID
	input.ShareType = &shareType.ID
	body, bodyErr := wireBody(struct {
		Share ShareCreateRequest `json:"share"`
	}{input})
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/shares", Body: body, Refs: []types.ResourceRef{network, shareType}, Options: options, Validate: func() error {
		if bindErr != nil {
			return bindErr
		}
		if e := c.validateRootSelection(network, "share_network", op); e != nil {
			return e
		}
		if e := c.validateRootSelection(shareType, "share_type", op); e != nil {
			return e
		}
		if originalNetwork != nil && *originalNetwork != network.ID || originalType != nil && *originalType != shareType.ID {
			return types.NewError(types.ScopeMismatch)
		}
		if bodyErr != nil || input.Validate() != nil || input.Size == nil || input.Size.String() == "" || input.Size.String() == "0" || strings.HasPrefix(input.Size.String(), "-") {
			return types.NewError(types.InvalidArgument)
		}
		if input.ShareProto == nil || (*input.ShareProto != "NFS" && *input.ShareProto != "CIFS") {
			return types.NewError(types.InvalidArgument)
		}
		if input.SnapshotID != nil || input.ShareGroupID != nil || input.SchedulerHints != nil {
			return types.NewError(types.CapabilityUnavailable)
		}
		if input.Metadata != nil {
			var m map[string]string
			if json.Unmarshal(input.Metadata.Reveal(), &m) != nil || m == nil {
				return types.NewError(types.InvalidArgument)
			}
		}
		return nil
	}}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[Share], error) {
		d := core.Decoded[Share]{}
		var env struct {
			Share json.RawMessage `json:"share"`
		}
		var identity struct {
			ID        *string `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if json.Unmarshal(raw, &env) != nil || json.Unmarshal(env.Share, &identity) != nil || identity.ID == nil || *identity.ID == "" || identity.ProjectID == nil {
			return d, types.NewError(types.ProtocolError)
		}
		if *identity.ProjectID != c.scope.ProjectID {
			return d, types.NewError(types.ScopeMismatch)
		}
		ref := core.Ref(ep, "filestorage", "share", *identity.ID)
		d.Refs = []types.ResourceRef{*ref}
		if json.Unmarshal(env.Share, &d.Data) != nil || d.Data.Validate() != nil {
			return d, types.NewError(types.ProtocolError)
		}
		for _, pair := range []struct {
			value    *types.Nullable[string]
			expected string
		}{{d.Data.ShareNetworkID, network.ID}, {d.Data.ShareType, shareType.ID}} {
			if pair.value != nil {
				if v, ok := pair.value.Value(); ok && v != pair.expected {
					return d, types.NewError(types.ScopeMismatch)
				}
			}
		}
		d.Data.Ref = ref
		return d, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
func (c *Client) validateRootSelection(ref types.ResourceRef, kind string, op core.Operation) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != c.scope || ref.Service != "filestorage" || ref.APIGeneration != op.APIGeneration || ref.Kind != kind || ref.Parent != nil || ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
