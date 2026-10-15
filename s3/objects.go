package s3

import (
	"context"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func versionQuery(v *types.ProtectedValue) url.Values {
	q := url.Values{}
	if v != nil {
		q.Set("versionId", string(v.Reveal()))
	}
	return q
}
func (c *Objects) Head(ctx context.Context, r types.ResourceRef, in HeadObjectOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	h := http.Header{}
	var encode error
	for _, v := range []struct {
		k string
		v *string
	}{{"If-Match", in.IfMatch}, {"If-None-Match", in.IfNoneMatch}, {"Range", in.Range}} {
		if v.v != nil {
			if e := header(h, v.k, *v.v); e != nil {
				encode = e
			}
		}
	}
	return invoke(c.client, ctx, "objects.head", "HEAD", r, versionQuery(in.VersionID), h, nil, []int{200, 206, 304}, o, func() error { return encode }, func(_ context.Context, b io.Reader, _ types.EndpointBinding, status int, h http.Header) (core.Decoded[ObjectInfo], error) {
		out := objectHeaders(h, r)
		out.NotModified = status == 304
		return core.Decoded[ObjectInfo]{Data: out}, emptyBody(b)
	})
}
func (c *Objects) Delete(ctx context.Context, r types.ResourceRef, in DeleteObjectOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	result, err := invoke(c.client, ctx, "objects.delete", "DELETE", r, versionQuery(in.VersionID), nil, nil, []int{204}, o, nil, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ObjectInfo], error) {
		return core.Decoded[ObjectInfo]{Data: mutationHeaders(h, r)}, emptyBody(b)
	})
	if result.Receipt != nil && in.VersionID != nil {
		result.Receipt.Recovery = "unavailable"
	}
	return result, err
}
func (c *Objects) Copy(ctx context.Context, source, target types.ResourceRef, in CopyOptions, o types.CallOptions) (types.Result[ObjectInfo], error) {
	s := c.client
	h, encode := uploadHeaders(UploadOptions{ContentType: in.ContentType, Metadata: in.Metadata, IfMatch: in.IfMatch, IfNoneMatch: in.IfNoneMatch})
	copySource := objectPath(source)
	if in.SourceVersionID != nil {
		copySource += "?versionId=" + strings.ReplaceAll(url.QueryEscape(string(in.SourceVersionID.Reveal())), "+", "%20")
	}
	if e := header(h, "X-Amz-Copy-Source", copySource); e != nil {
		encode = e
	}
	if in.MetadataDirective != nil {
		h.Set("X-Amz-Metadata-Directive", *in.MetadataDirective)
	}
	for _, v := range []struct {
		k string
		v *string
	}{{"X-Amz-Copy-Source-If-Match", in.SourceIfMatch}, {"X-Amz-Copy-Source-If-None-Match", in.SourceIfNoneMatch}} {
		if v.v != nil {
			if e := header(h, v.k, *v.v); e != nil {
				encode = e
			}
		}
	}
	return invoke(s, ctx, "objects.copy", "PUT", target, nil, h, nil, []int{200}, o, func() error {
		if encode != nil {
			return encode
		}
		if e := s.validObject(source); e != nil {
			return e
		}
		if e := s.currentEndpoint(source); e != nil {
			return e
		}
		if in.MetadataDirective != nil && *in.MetadataDirective != "COPY" && *in.MetadataDirective != "REPLACE" {
			return types.NewError(types.InvalidArgument)
		}
		if in.Metadata != nil && (in.MetadataDirective == nil || *in.MetadataDirective != "REPLACE") {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[ObjectInfo], error) {
		out := mutationHeaders(h, target)
		var body struct {
			XMLName      xml.Name
			ETag         *string `xml:"ETag"`
			LastModified *string `xml:"LastModified"`
			Code         string  `xml:"Code"`
		}
		if e := s.readXML(b, &body); e != nil {
			return core.Decoded[ObjectInfo]{Data: out}, e
		}
		if body.XMLName.Local == "Error" {
			e := types.NewError(types.ProviderError)
			if len(body.Code) < 129 {
				e.ProviderCode = body.Code
			}
			return core.Decoded[ObjectInfo]{Data: out}, e
		}
		if body.XMLName.Local != "CopyObjectResult" {
			return core.Decoded[ObjectInfo]{Data: out}, types.NewError(types.ProtocolError)
		}
		out.ETag = body.ETag
		out.LastModified = body.LastModified
		return core.Decoded[ObjectInfo]{Data: out}, nil
	})
}
