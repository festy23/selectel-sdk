package s3

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"strings"
	"unicode/utf8"
)

// PresignPost returns protected form inputs; the caller submits file last.
// The policy always pins this exact bucket/key and the explicit byte range.
func (c *Objects) PresignPost(ctx context.Context, ref types.ResourceRef, in PostOptions, options types.CallOptions) (types.Result[PresignedPost], error) {
	s := c.client
	op, bind := s.callOp("objects.post", "POST", 200, 201, 204)
	op = addressOperation(op, ref)
	fields := map[string]string{}
	var encode error
	if in.Conditions.ContentType != nil {
		value := string(in.Conditions.ContentType.Reveal())
		if !utf8.ValidString(value) {
			encode = types.NewError(types.InvalidArgument)
		}
		fields["Content-Type"] = value
	}
	for k, v := range in.Conditions.Metadata {
		value := string(v.Reveal())
		if k == "" || strings.ContainsAny(k, " :\t") || !utf8.ValidString(value) {
			encode = types.NewError(types.InvalidArgument)
		}
		fields["x-amz-meta-"+k] = value
	}
	// Reuse the public header validation boundary for supported form values, without
	// inserting PUT defaults into POST fields or signing unrequested fields.
	_, headerErr := uploadHeaders(UploadOptions{ContentType: in.Conditions.ContentType, Metadata: in.Conditions.Metadata})
	if headerErr != nil {
		encode = headerErr
	}
	bucket := ""
	path := ""
	if ref.Parent != nil {
		bucket = ref.Parent.ID
		path = bucketPath(*ref.Parent)
	}
	result, e := s.runtime.PresignS3Post(ctx, op, core.Request{Scope: s.scope, Path: path, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		return s.validObject(ref)
	}}, core.S3PostPolicy{Bucket: bucket, Key: ref.ID, MinBytes: in.MinBytes, MaxBytes: in.MaxBytes, Fields: fields}, in.Expires)
	return types.Result[PresignedPost]{Data: PresignedPost{URL: result.Data.URL, Fields: result.Data.Fields, Policy: result.Data.Policy, ExpiresAt: result.Data.ExpiresAt}, Metadata: result.Metadata}, e
}
