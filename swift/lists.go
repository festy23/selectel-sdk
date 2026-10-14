package swift

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/pagination"
	"github.com/festy23/selectel-sdk/types"
)

func listQuery(spec ListOptions) url.Values {
	q := url.Values{"format": {"json"}}
	if spec.Prefix != nil {
		q.Set("prefix", *spec.Prefix)
	}
	if spec.Delimiter != nil {
		q.Set("delimiter", *spec.Delimiter)
	}
	return q
}
func validList(spec ListOptions) error {
	for _, v := range []*string{spec.Marker, spec.Prefix, spec.Delimiter} {
		if v != nil && (!utf8.ValidString(*v) || strings.ContainsAny(*v, "\r\n\x00") || len(*v) > 1024) {
			return types.NewError(types.InvalidArgument)
		}
	}
	return nil
}
func collect[T, H any](c *Client, ctx context.Context, id, path string, ref *types.ResourceRef, spec ListOptions, options pagination.Options, header func(http.Header, types.EndpointBinding) (H, error), bind func(*T, types.EndpointBinding) (string, error)) (Listing[T, H], error) {
	out := Listing[T, H]{Collection: pagination.Collection[T]{Items: []T{}, StopReason: "error"}}
	op, e := c.operation(id, "GET", []int{200, 204})
	if e != nil {
		return out, e
	}
	if e = validList(spec); e != nil {
		return out, e
	}
	if ref != nil {
		if e = c.refError(*ref, "bucket"); e != nil {
			return out, e
		}
	}
	semantic := struct {
		Parent  *types.ResourceRef
		Filters ListOptions
	}{ref, spec}
	plan, e := c.runtime.PagePlan(id, "v1", c.scope, op.EndpointID, semantic)
	if e != nil {
		return out, e
	}
	plan.WireMax = 10000
	headers := []H{}
	fetch := func(ctx context.Context, cursor string, size uint) (pagination.SourcePage[T], error) {
		q := listQuery(spec)
		q.Set("limit", strconv.FormatUint(uint64(size), 10))
		marker := cursor
		if marker == "" && spec.Marker != nil {
			marker = *spec.Marker
		}
		if marker != "" {
			q.Set("marker", marker)
		}
		refs := []types.ResourceRef{}
		if ref != nil {
			refs = append(refs, *ref)
		}
		next := ""
		result, e := invoke(c, ctx, id, "GET", path, []int{200, 204}, refs, http.Header{"Accept": {"application/json"}}, nil, q, types.CallOptions{}, nil, func(_ context.Context, r io.Reader, ep types.EndpointBinding, status int, h http.Header) (core.Decoded[[]T], error) {
			info, e := header(h, ep)
			if e != nil {
				return core.Decoded[[]T]{}, e
			}
			headers = append(headers, info)
			rows := []T{}
			raw, e := c.control(r)
			if e != nil {
				return core.Decoded[[]T]{}, e
			}
			if status == 204 {
				if len(strings.TrimSpace(string(raw))) != 0 {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				return core.Decoded[[]T]{Data: rows}, nil
			}
			if !utf8.Valid(raw) {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			var wire []map[string]json.RawMessage
			if json.Unmarshal(raw, &wire) != nil || wire == nil {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			if len(wire) > int(size) {
				return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
			}
			previous := marker
			for _, fields := range wire {
				if fields == nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				delete(fields, "ref")
				encoded, _ := json.Marshal(fields)
				var row T
				if json.Unmarshal(encoded, &row) != nil {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				name, e := bind(&row, ep)
				if e != nil {
					return core.Decoded[[]T]{}, e
				}
				if name <= previous {
					return core.Decoded[[]T]{}, types.NewError(types.ProtocolError)
				}
				previous = name
				rows = append(rows, row)
			}
			next = previous
			return core.Decoded[[]T]{Data: rows}, nil
		})
		page := pagination.SourcePage[T]{Items: result.Data, Metadata: result.Metadata, Done: e == nil && len(result.Data) == 0}
		if !page.Done && e == nil {
			value := types.Protect([]byte(next))
			page.SensitiveNext = &value
		}
		return page, e
	}
	it, e := pagination.New(plan, options, fetch)
	if e != nil {
		return out, e
	}
	out.Collection, e = it.Collect(ctx)
	out.HeaderPages = headers
	return out, e
}

// Buckets retains typed per-page account headers and a scope/filter-bound cursor.
// Empty pages prove exhaustion; short/clamped pages do not imply completeness.
func (c *Client) Buckets(ctx context.Context, spec ListOptions, options pagination.Options) (Listing[BucketEntry, AccountInfo], error) {
	return collect(c, ctx, "swift.v1.buckets.list", accountPath(c.scope), nil, spec, options, func(h http.Header, ep types.EndpointBinding) (AccountInfo, error) {
		return accountInfo(h, accountRef(ep))
	}, func(row *BucketEntry, ep types.EndpointBinding) (string, error) {
		if row.Name == nil || !validName(*row.Name) || strings.Contains(*row.Name, "/") {
			return "", types.NewError(types.ProtocolError)
		}
		row.Ref = bucketRef(ep, *row.Name)
		return *row.Name, nil
	})
}
func (c *Client) Objects(ctx context.Context, bucket types.ResourceRef, spec ListOptions, options pagination.Options) (Listing[ObjectEntry, BucketInfo], error) {
	return collect(c, ctx, "swift.v1.objects.list", refPath(bucket), &bucket, spec, options, func(h http.Header, _ types.EndpointBinding) (BucketInfo, error) {
		r := cloneRef(bucket)
		return bucketInfo(h, &r)
	}, func(row *ObjectEntry, _ types.EndpointBinding) (string, error) {
		if (row.Name == nil) == (row.Subdir == nil) {
			return "", types.NewError(types.ProtocolError)
		}
		name := row.Name
		if name == nil {
			name = row.Subdir
		}
		if !validName(*name) {
			return "", types.NewError(types.ProtocolError)
		}
		row.Ref = nil
		if row.Name != nil {
			row.Ref = objectRef(bucket, *row.Name)
		}
		return *name, nil
	})
}
