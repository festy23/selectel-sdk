package support

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
	"net/url"
	"strconv"
	"unicode/utf8"
)

type AttachmentListOptions struct {
	ActorUUID, FileUUID *string
	EmployeeID          *types.Integer
	FileHash            *types.ProtectedValue
	ExternalIDs         *[]types.ProtectedValue
	UserIDs             *[]types.Integer
	State, Visibility   *types.ProtectedValue
	Page                *types.Integer
}

func int32Value(value types.Integer) error {
	n, ok := new(big.Int).SetString(value.String(), 10)
	if !ok || !n.IsInt64() || n.Int64() < -2147483648 || n.Int64() > 2147483647 {
		return types.NewError(types.InvalidArgument)
	}
	return nil
}
func attachmentQuery(spec AttachmentListOptions) (url.Values, error) {
	q := url.Values{}
	var result error
	for key, v := range map[string]*string{"actor_uuid": spec.ActorUUID, "file_uuid": spec.FileUUID} {
		if v != nil {
			if !fileUUID.MatchString(*v) {
				result = types.NewError(types.InvalidArgument)
			}
			q.Set(key, *v)
		}
	}
	for key, v := range map[string]*types.ProtectedValue{"file_hash": spec.FileHash, "state": spec.State, "visibility": spec.Visibility} {
		if v != nil {
			if !utf8.Valid(v.Reveal()) {
				result = types.NewError(types.InvalidArgument)
			}
			q.Set(key, string(v.Reveal()))
		}
	}
	if spec.EmployeeID != nil {
		if e := int32Value(*spec.EmployeeID); e != nil {
			result = e
		}
		q.Set("employee_id", spec.EmployeeID.String())
	}
	if spec.UserIDs != nil {
		for _, v := range *spec.UserIDs {
			if e := int32Value(v); e != nil {
				result = e
			}
			q.Add("user_id", v.String())
		}
	}
	if spec.ExternalIDs != nil {
		for _, v := range *spec.ExternalIDs {
			if !utf8.Valid(v.Reveal()) {
				result = types.NewError(types.InvalidArgument)
			}
			q.Add("external_id", string(v.Reveal()))
		}
	}
	q.Set("page", "1")
	if spec.Page != nil {
		if e := int32Value(*spec.Page); e != nil {
			result = e
		}
		n, ok := new(big.Int).SetString(spec.Page.String(), 10)
		if !ok || n.Sign() < 1 {
			result = types.NewError(types.InvalidArgument)
		}
		q.Set("page", spec.Page.String())
	}
	return q, result
}

// List preserves the available page and partial replay cursor. The source has
// no total/next metadata or terminal rule, so a fully issued page is incomplete.
func (c *AttachmentsClient) List(ctx context.Context, spec AttachmentListOptions, options pagination.Options) (pagination.Collection[AttachmentUploadedFile], error) {
	op, bindErr := c.operation("tickets.attachments_v1.files.list", "GET")
	query, queryErr := attachmentQuery(spec)
	if bindErr != nil {
		return pagination.Collection[AttachmentUploadedFile]{}, bindErr
	}
	if queryErr != nil {
		return pagination.Collection[AttachmentUploadedFile]{}, queryErr
	}
	plan, e := c.runtime.PagePlan(op.ID, "attachments_v1", c.scope, op.EndpointID, query.Encode())
	if e != nil {
		return pagination.Collection[AttachmentUploadedFile]{}, e
	}
	plan.WireMax = 100
	fetch := func(ctx context.Context, input string, size uint) (pagination.SourcePage[AttachmentUploadedFile], error) {
		q := url.Values{}
		for key, v := range query {
			q[key] = append([]string(nil), v...)
		}
		q.Set("page_size", strconv.FormatUint(uint64(size), 10))
		out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/public/files/list", Query: q}, func(raw []byte, ep types.EndpointBinding) (core.Decoded[[]AttachmentUploadedFile], error) {
			decoded := core.Decoded[[]AttachmentUploadedFile]{}
			if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data == nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			seen := map[string]bool{}
			for i := range decoded.Data {
				row := &decoded.Data[i]
				if row.FileUUID != nil {
					if !fileUUID.MatchString(*row.FileUUID) {
						return decoded, types.NewError(types.ProtocolError)
					}
					if spec.FileUUID != nil && *row.FileUUID != *spec.FileUUID {
						return decoded, types.NewError(types.ScopeMismatch)
					}
					if seen[*row.FileUUID] {
						return decoded, types.NewError(types.ProtocolError)
					}
					seen[*row.FileUUID] = true
					row.Ref = core.Ref(ep, "tickets", "file", *row.FileUUID)
				}
			}
			return decoded, nil
		})
		return pagination.SourcePage[AttachmentUploadedFile]{Items: out.Data, Metadata: out.Metadata, Next: input}, e
	}
	it, e := pagination.New(plan, options, fetch)
	if e != nil {
		return pagination.Collection[AttachmentUploadedFile]{}, e
	}
	return it.Collect(ctx)
}
