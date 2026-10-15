package filestorage

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"unicode/utf8"
)

// ListAccessRules observes the pinned modern representation; it does not fall
// back to the pre-2.45 access_list action or imply deployed driver support.
func (c *Client) ListAccessRules(ctx context.Context, share types.ResourceRef, metadata *types.ProtectedJSON, options pagination.Options) (pagination.Collection[AccessRule], error) {
	op := core.Operation{ID: "filestorage.v2_73.share_access_rules.list", Service: "filestorage", APIGeneration: "v2_73", Method: http.MethodGet, Effect: "read", Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: []int{200}, Headers: http.Header{"X-OpenStack-Manila-API-Version": []string{"2.73"}}}
	op, e := c.runtime.BindDocumented(op, c.scope)
	if e != nil {
		return pagination.Collection[AccessRule]{}, e
	}
	if e = c.validateShare(share, op); e != nil {
		return pagination.Collection[AccessRule]{}, e
	}
	q := url.Values{"share_id": []string{share.ID}}
	if metadata != nil {
		raw := metadata.Reveal()
		var v map[string]string
		if !utf8.Valid(raw) || json.Unmarshal(raw, &v) != nil || v == nil {
			return pagination.Collection[AccessRule]{}, types.NewError(types.InvalidArgument)
		}
		encoded, _ := json.Marshal(v)
		q.Set("metadata", string(encoded))
	}
	iterator, e := core.FixedIterator(c.runtime, op, core.Request{Scope: c.scope, Path: "/share-access-rules", Query: q}, q.Encode(), options, "access_list", func(v *AccessRule, ep types.EndpointBinding) error {
		if v.Validate() != nil || v.ID == nil || *v.ID == "" {
			return types.NewError(types.ProtocolError)
		}
		if v.ShareID != nil && *v.ShareID != share.ID {
			return types.NewError(types.ScopeMismatch)
		}
		v.Ref = core.Ref(ep, "filestorage", "access_rule", *v.ID)
		parent := cloneRef(share)
		v.Ref.Parent = &parent
		return nil
	})
	if e != nil {
		return pagination.Collection[AccessRule]{}, e
	}
	return iterator.Collect(ctx)
}
func (c *Client) validateShare(ref types.ResourceRef, op core.Operation) error {
	if ref.Validate() != nil {
		return types.NewError(types.InvalidArgument)
	}
	if ref.Scope != c.scope || ref.Service != "filestorage" || ref.APIGeneration != op.APIGeneration || ref.Kind != "share" || ref.Parent != nil {
		return types.NewError(types.ScopeMismatch)
	}
	if ref.EndpointID != op.EndpointID || ref.EndpointURL != op.FixedEndpoint {
		return types.NewError(types.ScopeMismatch)
	}
	return nil
}
