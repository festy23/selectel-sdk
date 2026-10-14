package swift

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// BulkDelete submits only the explicitly supplied object names. A successful HTTP
// response can contain per-object failures; those retain the accepted receipt.
func (c *Client) BulkDelete(ctx context.Context, refs []types.ResourceRef, o types.CallOptions) (types.Result[BulkResult], error) {
	var body []byte
	known := map[string]bool{}
	var preflight error
	if len(refs) == 0 || len(refs) > 10000 {
		preflight = types.NewError(types.InvalidArgument)
	}
	paths := make([]string, 0, min(len(refs), 10000))
	if preflight == nil {
		for _, ref := range refs {
			if e := c.refError(ref, "object"); e != nil {
				preflight = e
				break
			}
			key := ref.Parent.ID + "/" + ref.ID
			if known[key] {
				preflight = types.NewError(types.InvalidArgument)
				break
			}
			known[key] = true
			paths = append(paths, relativeObject(ref, false))
		}
		body = []byte(strings.Join(paths, "\n"))
		if uint64(len(body)) > c.runtime.StructuredLimit() {
			preflight = types.NewError(types.InvalidArgument)
		}
	}
	return invoke(c, ctx, "swift.v1.objects.bulk_delete", "POST", accountPath(c.scope), []int{200}, refs, http.Header{"Content-Type": {"text/plain"}}, body, url.Values{"bulk-delete": {"true"}, "format": {"json"}}, o, func() error { return preflight }, func(_ context.Context, r io.Reader, _ types.EndpointBinding, _ int, _ http.Header) (core.Decoded[BulkResult], error) {
		raw, e := c.control(r)
		if e != nil {
			return core.Decoded[BulkResult]{}, e
		}
		var wire struct {
			Deleted *types.Integer `json:"Number Deleted"`
			Missing *types.Integer `json:"Number Not Found"`
			Status  *string        `json:"Response Status"`
			Body    *string        `json:"Response Body"`
			Errors  [][]string     `json:"Errors"`
		}
		if json.Unmarshal(raw, &wire) != nil {
			return core.Decoded[BulkResult]{}, types.NewError(types.ProtocolError)
		}
		out := BulkResult{NumberDeleted: wire.Deleted, NumberNotFound: wire.Missing}
		if wire.Status != nil {
			v := types.Protect([]byte(*wire.Status))
			out.ResponseStatus = &v
		}
		if wire.Body != nil {
			v := types.Protect([]byte(*wire.Body))
			out.ResponseBody = &v
		}
		var decodeErr error
		for _, row := range wire.Errors {
			if len(row) != 2 {
				decodeErr = types.NewError(types.ProtocolError)
				continue
			}
			out.Errors = append(out.Errors, BulkError{Path: types.Protect([]byte(row[0])), Detail: types.Protect([]byte(row[1]))})
			path, x := url.PathUnescape(strings.TrimPrefix(row[0], "/"))
			if x != nil || !known[path] {
				decodeErr = types.NewError(types.ScopeMismatch)
			}
		}
		if wire.Deleted == nil || wire.Missing == nil || strings.HasPrefix(wire.Deleted.String(), "-") || strings.HasPrefix(wire.Missing.String(), "-") || wire.Status == nil || len(*wire.Status) < 3 {
			decodeErr = types.NewError(types.ProtocolError)
		}
		if decodeErr == nil {
			status, x := strconv.Atoi((*wire.Status)[:3])
			if x != nil || status < 100 || status > 599 {
				decodeErr = types.NewError(types.ProtocolError)
			} else if status < 200 || status >= 300 || len(out.Errors) > 0 {
				decodeErr = types.NewError(types.OperationFailed)
			}
		}
		return core.Decoded[BulkResult]{Data: out}, decodeErr
	})
}

// DeleteObject deletes the addressed object only. Segment cleanup is explicit
// through BulkDelete, with separately supplied segment references.
func (c *Client) DeleteObject(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ObjectInfo], error) {
	return c.objectControl(ctx, "swift.v1.objects.delete", "DELETE", nil, ref, nil, nil, nil, nil, []int{204}, o, nil)
}

// DeleteSegments explicitly requests deletion of the manifest and its segments.
// Only the manifest locator is known; acknowledgement does not enumerate or
// establish absence of individual segments.
func (c *Client) DeleteSegments(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[ObjectInfo], error) {
	return c.objectControl(ctx, "swift.v1.objects.delete", "DELETE", nil, ref, nil, nil, nil, url.Values{"multipart-manifest": {"delete"}}, []int{204}, o, nil)
}
