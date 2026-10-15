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
	fields, encode := postFields(in.Conditions)
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

// PresignPostTemplate binds a filename form to one bucket and explicit prefix.
// It returns no exact object ref or proof of the eventual object name/creation.
func (c *Buckets) PresignPostTemplate(ctx context.Context, ref types.ResourceRef, in PostTemplateOptions, options types.CallOptions) (types.Result[PresignedPost], error) {
	s := c.client
	op, bind := s.callOp("objects.post_template", "POST", 200, 201, 204)
	op = addressOperation(op, ref)
	fields, encode := postFields(in.Conditions)
	result, e := s.runtime.PresignS3PostTemplate(ctx, op, core.Request{Scope: s.scope, Path: bucketPath(ref), Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Options: options, Validate: func() error {
		if bind != nil {
			return bind
		}
		if encode != nil {
			return encode
		}
		return s.valid(ref, "bucket_name")
	}}, core.S3PostPolicy{Bucket: ref.ID, Prefix: in.Prefix, MinBytes: in.MinBytes, MaxBytes: in.MaxBytes, Fields: fields}, in.Expires)
	return types.Result[PresignedPost]{Data: PresignedPost{URL: result.Data.URL, Fields: result.Data.Fields, Policy: result.Data.Policy, ExpiresAt: result.Data.ExpiresAt}, Metadata: result.Metadata}, e
}

func postFields(in PostConditions) (map[string]string, error) {
	fields := map[string]string{}
	var encode error
	if in.ContentType != nil {
		value := string(in.ContentType.Reveal())
		if !utf8.ValidString(value) {
			encode = types.NewError(types.InvalidArgument)
		}
		fields["Content-Type"] = value
	}
	for k, v := range in.Metadata {
		value := string(v.Reveal())
		if k == "" || strings.ContainsAny(k, " :\t") || !utf8.ValidString(value) {
			encode = types.NewError(types.InvalidArgument)
		}
		fields["x-amz-meta-"+k] = value
	}
	// Reuse the public header validation boundary for supported form values, without
	// inserting PUT defaults into POST fields or signing unrequested fields.
	_, headerErr := uploadHeaders(UploadOptions{ContentType: in.ContentType, Metadata: in.Metadata})
	if headerErr != nil {
		encode = headerErr
	}
	return fields, encode
}
