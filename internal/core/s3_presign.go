package core

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type S3Presigned struct {
	URL       types.ProtectedValue            `json:"url"`
	Headers   map[string]types.ProtectedValue `json:"headers"`
	ExpiresAt time.Time                       `json:"expires_at"`
}

// PresignS3 is a private compiled binding; it never executes a cloud mutation.
func (r *Runtime) PresignS3(ctx context.Context, op Operation, in Request, expires time.Duration) (out types.Result[S3Presigned], err error) {
	out.Metadata = types.Metadata{Scope: in.Scope, APIGeneration: op.APIGeneration}
	defer func() { out.Metadata.ObservedAt = time.Now().UTC() }()
	if r == nil || ctx == nil || expires <= 0 || expires > 15*time.Minute || expires%time.Second != 0 || op.Service != "s3" || op.APIGeneration != "v2006_03_01" || op.Auth != types.S3SigV4 || op.SigV4Service != "s3" || !((op.ID == "s3.v2006_03_01.objects.get" && op.Method == "GET") || (op.ID == "s3.v2006_03_01.objects.put" && op.Method == "PUT")) {
		return out, types.NewError(types.InvalidArgument)
	}
	call, cancel := context.WithTimeout(ctx, r.config.CallTimeout)
	defer cancel()
	if call.Err() != nil {
		return out, types.ContextError(call.Err())
	}
	if in.Scope.Validate() != nil || in.Scope.ProjectID == "" || in.Scope.Pool == "" {
		return out, types.NewError(types.InvalidArgument)
	}
	if in.Scope.AccountID != r.config.AccountID {
		return out, types.NewError(types.ScopeMismatch)
	}
	if in.Options.IdempotencyKey != "" || in.Options.AuthMode != "" && in.Options.AuthMode != types.S3SigV4 {
		return out, types.NewError(types.InvalidArgument)
	}
	if _, e := r.catalog.Describe(op.ID); e != nil {
		return out, e
	}
	if in.Validate != nil {
		if e := in.Validate(); e != nil {
			return out, e
		}
	}
	if !strings.HasPrefix(in.Path, "/") || strings.ContainsAny(in.Path, "?#") {
		return out, types.NewError(types.InvalidArgument)
	}
	for _, ref := range in.Refs {
		if ref.Validate() != nil || ref.Scope != in.Scope || ref.Service != "s3" || ref.APIGeneration != op.APIGeneration {
			return out, types.NewError(types.ScopeMismatch)
		}
	}
	entry, e := r.authorize(call, types.AuthRequest{Scope: in.Scope, Mode: types.S3SigV4, Service: "s3", Audience: "selectel"})
	if e != nil {
		return out, e
	}
	ep, e := r.resolve(op, in, entry)
	if e != nil {
		return out, e
	}
	out.Metadata.EndpointID = ep.ID
	now := time.Now().UTC().Truncate(time.Second)
	until := now.Add(expires)
	if !entry.material.ExpiresAt.IsZero() && !until.Before(entry.material.ExpiresAt) {
		return out, types.NewError(types.Unauthenticated)
	}
	u, e := url.Parse(strings.TrimRight(ep.URL, "/") + in.Path)
	if e != nil {
		return out, types.NewError(types.InvalidArgument)
	}
	query := url.Values{}
	for k, v := range in.Query {
		query[k] = append([]string(nil), v...)
	}
	query.Set("X-Amz-Expires", strconv.FormatInt(int64(expires/time.Second), 10))
	u.RawQuery = query.Encode()
	req, e := http.NewRequestWithContext(call, op.Method, u.String(), nil)
	if e != nil {
		return out, types.NewError(types.InvalidArgument)
	}
	for k, v := range op.Headers {
		if strings.EqualFold(k, "Authorization") || strings.EqualFold(k, "Host") || strings.HasPrefix(strings.ToLower(k), "x-amz-signature") {
			return out, types.NewError(types.InvalidArgument)
		}
		req.Header[k] = append([]string(nil), v...)
	}
	material := aws.Credentials{AccessKeyID: string(entry.material.AccessKey.Reveal()), SecretAccessKey: string(entry.material.SecretKey.Reveal()), SessionToken: string(entry.material.SessionToken.Reveal())}
	signed, headers, e := v4.NewSigner().PresignHTTP(call, material, req, "UNSIGNED-PAYLOAD", "s3", in.Scope.Pool, now, func(o *v4.SignerOptions) { o.DisableURIPathEscaping = true; o.DisableHeaderHoisting = true })
	if e != nil {
		return out, types.NewError(types.Unauthenticated)
	}
	if call.Err() != nil {
		return out, types.ContextError(call.Err())
	}
	out.Data = S3Presigned{URL: types.Protect([]byte(signed)), Headers: map[string]types.ProtectedValue{}, ExpiresAt: until}
	for k, v := range headers {
		out.Data.Headers[k] = types.Protect([]byte(strings.Join(v, ",")))
	}
	return out, nil
}
