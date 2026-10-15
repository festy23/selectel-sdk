package s3

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"time"
)

type PresignOptions struct {
	Expires                     time.Duration
	VersionID                   *types.ProtectedValue
	Range, IfMatch, IfNoneMatch *string
}

func (c *Objects) presign(ctx context.Context, r types.ResourceRef, in PresignOptions, h http.Header, action, method string, o types.CallOptions, encode error) (types.Result[core.S3Presigned], error) {
	s := c.client
	op, bind := s.callOp(action, method, 200)
	op = addressOperation(op, r)
	op.Headers = h
	return s.runtime.PresignS3(ctx, op, core.Request{Scope: s.scope, Path: objectWirePath(r), Query: versionQuery(in.VersionID), Refs: []types.ResourceRef{r}, PinnedEndpoint: pin(r), Options: o, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		return s.validObject(r)
	}}, in.Expires)
}
func (c *Objects) PresignGet(ctx context.Context, r types.ResourceRef, in PresignOptions, o types.CallOptions) (types.Result[PresignedURL], error) {
	h := http.Header{}
	var encode error
	for _, v := range []struct {
		k string
		v *string
	}{{"Range", in.Range}, {"If-Match", in.IfMatch}, {"If-None-Match", in.IfNoneMatch}} {
		if v.v != nil {
			if e := header(h, v.k, *v.v); e != nil {
				encode = e
			}
		}
	}
	v, e := c.presign(ctx, r, in, h, "objects.get", "GET", o, encode)
	return types.Result[PresignedURL]{Data: PresignedURL{URL: v.Data.URL, Headers: v.Data.Headers, ExpiresAt: v.Data.ExpiresAt}, Metadata: v.Metadata}, e
}
func (c *Objects) PresignPut(ctx context.Context, r types.ResourceRef, in PresignOptions, upload UploadOptions, o types.CallOptions) (types.Result[PresignedURL], error) {
	h, encode := uploadHeaders(upload)
	if in.VersionID != nil || in.Range != nil || in.IfMatch != nil || in.IfNoneMatch != nil {
		encode = types.NewError(types.InvalidArgument)
	}
	v, e := c.presign(ctx, r, in, h, "objects.put", "PUT", o, encode)
	return types.Result[PresignedURL]{Data: PresignedURL{URL: v.Data.URL, Headers: v.Data.Headers, ExpiresAt: v.Data.ExpiresAt}, Metadata: v.Metadata}, e
}
