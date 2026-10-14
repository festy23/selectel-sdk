package swift

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func (c *Client) CreateBucket(ctx context.Context, name string, spec BucketCreate, o types.CallOptions) (types.Result[BucketInfo], error) {
	h, encodeErr := metadataRequest("X-Container-Meta-", spec.Metadata, nil)
	known, refErr := c.Bucket(name)
	refs := []types.ResourceRef{}
	if refErr == nil {
		refs = append(refs, known)
	} else {
		encodeErr = refErr
	}
	if spec.Type != nil {
		h.Set("X-Container-Meta-Type", *spec.Type)
	}
	if spec.StoragePolicy != nil {
		if e := setText(h, "X-Storage-Policy", spec.StoragePolicy); e != nil {
			encodeErr = e
		}
	}
	setBoolean(h, "X-Versions-Enabled", spec.VersionsEnabled)
	if e := setInteger(h, "X-Container-Meta-Default-Delete-After", spec.DefaultDeleteAfter); e != nil {
		encodeErr = e
	}
	return invoke(c, ctx, "swift.v1.buckets.create", "PUT", accountPath(c.scope)+"/"+url.PathEscape(name), []int{201}, refs, h, nil, nil, o, func() error {
		if encodeErr != nil {
			return encodeErr
		}
		if !validName(name) || strings.Contains(name, "/") {
			return types.NewError(types.InvalidArgument)
		}
		if spec.Type != nil && *spec.Type != "public" && *spec.Type != "private" {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}, func(_ context.Context, r io.Reader, ep types.EndpointBinding, _ int, h http.Header) (core.Decoded[BucketInfo], error) {
		ref := bucketRef(ep, name)
		data, e := bucketInfo(h, ref)
		if e == nil {
			e = c.discard(r)
		}
		return core.Decoded[BucketInfo]{Data: data, Refs: []types.ResourceRef{*ref}}, e
	})
}
func (c *Client) BucketInfo(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[BucketInfo], error) {
	return c.bucketCall(ctx, ref, "swift.v1.buckets.get", "HEAD", []int{204}, nil, nil, o)
}
func (c *Client) DeleteBucket(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[BucketInfo], error) {
	return c.bucketCall(ctx, ref, "swift.v1.buckets.delete", "DELETE", []int{204}, nil, nil, o)
}
func (c *Client) UpdateBucket(ctx context.Context, ref types.ResourceRef, spec BucketUpdate, o types.CallOptions) (types.Result[BucketInfo], error) {
	h, e := metadataRequest("X-Container-Meta-", spec.Metadata.Set, spec.Metadata.Remove)
	if spec.Type != nil {
		if h.Get("X-Remove-Container-Meta-Type") != "" {
			e = types.NewError(types.InvalidArgument)
		}
		h.Set("X-Container-Meta-Type", *spec.Type)
		if *spec.Type != "public" && *spec.Type != "private" {
			e = types.NewError(types.InvalidArgument)
		}
	}
	setBoolean(h, "X-Versions-Enabled", spec.VersionsEnabled)
	for _, f := range []struct {
		k string
		v *types.Integer
	}{{"X-Container-Meta-Quota-Bytes", spec.QuotaBytes}, {"X-Container-Meta-Quota-Count", spec.QuotaCount}, {"X-Container-Meta-Default-Delete-After", spec.DefaultDeleteAfter}} {
		if x := setInteger(h, f.k, f.v); x != nil {
			e = x
		}
	}
	return c.bucketCall(ctx, ref, "swift.v1.buckets.update", "POST", []int{204, 202}, h, e, o)
}
func (c *Client) bucketCall(ctx context.Context, ref types.ResourceRef, id, method string, status []int, h http.Header, encodeErr error, o types.CallOptions) (types.Result[BucketInfo], error) {
	return invoke(c, ctx, id, method, refPath(ref), status, []types.ResourceRef{ref}, h, nil, nil, o, func() error {
		if encodeErr != nil {
			return encodeErr
		}
		return c.refError(ref, "bucket")
	}, func(_ context.Context, r io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[BucketInfo], error) {
		known := cloneRef(ref)
		out, e := bucketInfo(h, &known)
		if e == nil {
			e = c.discard(r)
		}
		return core.Decoded[BucketInfo]{Data: out}, e
	})
}
func (c *Client) UpdateAccount(ctx context.Context, spec AccountUpdate, o types.CallOptions) (types.Result[AccountInfo], error) {
	h, e := metadataRequest("X-Account-Meta-", spec.Metadata.Set, spec.Metadata.Remove)
	if x := setInteger(h, "X-Account-Meta-Quota-Bytes", spec.QuotaBytes); x != nil {
		e = x
	}
	op, _ := c.operation("swift.v1.account.update", "POST", []int{204, 202})
	refs := []types.ResourceRef{}
	if op.FixedEndpoint != "" {
		refs = append(refs, *accountRef(types.EndpointBinding{ID: op.EndpointID, URL: op.FixedEndpoint, APIGeneration: "v1", Scope: c.scope}))
	}
	return invoke(c, ctx, "swift.v1.account.update", "POST", accountPath(c.scope), []int{204, 202}, refs, h, nil, nil, o, func() error { return e }, func(_ context.Context, r io.Reader, ep types.EndpointBinding, _ int, h http.Header) (core.Decoded[AccountInfo], error) {
		ref := accountRef(ep)
		out, e := accountInfo(h, ref)
		if e == nil {
			e = c.discard(r)
		}
		return core.Decoded[AccountInfo]{Data: out, Refs: []types.ResourceRef{*ref}}, e
	})
}
