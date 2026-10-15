package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/festy23/selectel-sdk/types"
)

// S3PostPolicy is a private, compiled POST form subset, not caller policy JSON.
// Bucket/key are exact refs; only content type and user metadata may add equality
// conditions. The authentication, key and length constraints cannot be replaced.
type S3PostPolicy struct {
	Bucket, Key        string
	MinBytes, MaxBytes int64
	Fields             map[string]string
}

type S3PostForm struct {
	URL, Policy types.ProtectedValue
	Fields      map[string]types.ProtectedValue
	ExpiresAt   time.Time
}

// PresignS3Post creates form inputs locally. The signature algorithm and canonical
// policy shape follow pinned AWS service/s3 presign_post.go; no service client,
// default credential chain, HTTP call or public raw signing seam is introduced.
func (r *Runtime) PresignS3Post(ctx context.Context, op Operation, in Request, policy S3PostPolicy, expires time.Duration) (out types.Result[S3PostForm], err error) {
	out.Metadata = types.Metadata{Scope: in.Scope, APIGeneration: op.APIGeneration}
	defer func() { out.Metadata.ObservedAt = time.Now().UTC() }()
	if r == nil || ctx == nil || op.ID != "s3.v2006_03_01.objects.post" || op.Service != "s3" || op.APIGeneration != "v2006_03_01" || op.Auth != types.S3SigV4 || op.SigV4Service != "s3" || op.Method != "POST" || expires <= 0 || expires > 15*time.Minute || expires%time.Second != 0 || policy.MinBytes < 0 || policy.MaxBytes <= 0 || policy.MinBytes > policy.MaxBytes {
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
	if len(in.Refs) != 1 || in.Refs[0].Validate() != nil || in.Refs[0].Scope != in.Scope || in.Refs[0].Service != "s3" || in.Refs[0].APIGeneration != op.APIGeneration || in.Refs[0].Parent == nil || policy.Key != in.Refs[0].ID || policy.Bucket != in.Refs[0].Parent.ID || strings.Contains(policy.Key, "${") {
		return out, types.NewError(types.ScopeMismatch)
	}
	if !strings.HasPrefix(in.Path, "/") || strings.ContainsAny(in.Path, "?#") {
		return out, types.NewError(types.InvalidArgument)
	}
	total := len(policy.Bucket) + len(policy.Key)
	for k, v := range policy.Fields {
		if !(k == "Content-Type" || strings.HasPrefix(k, "x-amz-meta-") && len(k) > len("x-amz-meta-")) || !utf8.ValidString(v) {
			return out, types.NewError(types.InvalidArgument)
		}
		total += len(k) + len(v)
		if uint64(total) > r.config.MaxJSONBytes {
			return out, types.NewError(types.InvalidArgument)
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
	u, e := url.Parse(strings.TrimRight(ep.URL, "/") + in.Path)
	if e != nil || u.Scheme != "https" {
		return out, types.NewError(types.InvalidArgument)
	}
	now := time.Now().UTC().Truncate(time.Second)
	until := now.Add(expires)
	if !entry.material.ExpiresAt.IsZero() && !until.Before(entry.material.ExpiresAt) {
		return out, types.NewError(types.Unauthenticated)
	}
	fields := map[string]string{"key": policy.Key, "X-Amz-Algorithm": "AWS4-HMAC-SHA256", "X-Amz-Date": now.Format("20060102T150405Z"), "X-Amz-Credential": string(entry.material.AccessKey.Reveal()) + "/" + now.Format("20060102") + "/" + in.Scope.Pool + "/s3/aws4_request"}
	if !entry.material.SessionToken.Empty() {
		fields["X-Amz-Security-Token"] = string(entry.material.SessionToken.Reveal())
	}
	for k, v := range policy.Fields {
		fields[k] = v
	}
	conditions := []any{map[string]string{"bucket": policy.Bucket}, []any{"content-length-range", policy.MinBytes, policy.MaxBytes}}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		conditions = append(conditions, map[string]string{k: fields[k]})
	}
	raw, e := json.Marshal(struct {
		Expiration string `json:"expiration"`
		Conditions []any  `json:"conditions"`
	}{until.Format(time.RFC3339), conditions})
	if e != nil || uint64(base64.StdEncoding.EncodedLen(len(raw))) > r.config.MaxJSONBytes {
		return out, types.NewError(types.InvalidArgument)
	}
	encoded := base64.StdEncoding.EncodeToString(raw)
	mac := func(key []byte, value string) []byte {
		h := hmac.New(sha256.New, key)
		_, _ = h.Write([]byte(value))
		return h.Sum(nil)
	}
	signingKey := mac(mac(mac(mac([]byte("AWS4"+string(entry.material.SecretKey.Reveal())), now.Format("20060102")), in.Scope.Pool), "s3"), "aws4_request")
	fields["policy"] = encoded
	fields["X-Amz-Signature"] = hex.EncodeToString(mac(signingKey, encoded))
	if call.Err() != nil {
		return out, types.ContextError(call.Err())
	}
	out.Data = S3PostForm{URL: types.Protect([]byte(u.String())), Policy: types.Protect([]byte(encoded)), Fields: map[string]types.ProtectedValue{}, ExpiresAt: until}
	for k, v := range fields {
		out.Data.Fields[k] = types.Protect([]byte(v))
	}
	return out, nil
}
