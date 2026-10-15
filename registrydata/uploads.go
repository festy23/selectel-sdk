package registrydata

import (
	"context"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Uploads struct{ client *Client }

func repositoryPath(repo types.ResourceRef) string {
	if repo.Parent == nil {
		return ""
	}
	return "/v2/" + repo.Parent.ID + "/" + repo.ID
}

// uploadLocation accepts a server-provided session address only in the exact
// repository and endpoint. Query state stays protected, never in a ResourceRef.
func (c *Uploads) uploadLocation(raw string, repo types.ResourceRef, expected *types.ResourceRef) (UploadSession, error) {
	out := UploadSession{}
	if uint64(len(raw)) > c.client.runtime.StructuredLimit() {
		return out, types.NewError(types.ResponseLimit)
	}
	base, e := url.Parse(repo.EndpointURL)
	if e != nil {
		return out, types.NewError(types.ProtocolError)
	}
	value, e := url.Parse(raw)
	if e != nil || raw == "" || value.User != nil || value.Fragment != "" || value.Opaque != "" {
		return out, types.NewError(types.ProtocolError)
	}
	requestURL, _ := url.Parse(strings.TrimRight(repo.EndpointURL, "/") + repositoryPath(repo) + "/blobs/uploads/")
	if expected != nil {
		requestURL.Path += expected.ID
		requestURL.RawPath = ""
	}
	value = requestURL.ResolveReference(value)
	if !sameOrigin(base, value) {
		return out, types.NewError(types.ProtocolError)
	}
	prefix := strings.TrimRight(base.Path, "/") + repositoryPath(repo) + "/blobs/uploads/"
	escapedPrefix := strings.TrimRight(base.EscapedPath(), "/") + repositoryPath(repo) + "/blobs/uploads/"
	if !strings.HasPrefix(value.Path, prefix) {
		return out, types.NewError(types.ScopeMismatch)
	}
	id := strings.TrimPrefix(value.Path, prefix)
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\\x00\r\n") || value.EscapedPath() != escapedPrefix+url.PathEscape(id) {
		return out, types.NewError(types.ProtocolError)
	}
	ref := child(repo, "upload", id)
	if expected != nil && expected.ID != id {
		return out, types.NewError(types.ScopeMismatch)
	}
	if values, e := url.ParseQuery(value.RawQuery); e != nil || values.Has("digest") {
		return out, types.NewError(types.ProtocolError)
	}
	out.Ref = ref
	out.Location = types.Protect([]byte(value.String()))
	return out, nil
}
func (c *Uploads) Start(ctx context.Context, repo types.ResourceRef, o types.CallOptions) (types.Result[UploadSession], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.start")
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: repositoryPath(repo) + "/blobs/uploads/", Refs: []types.ResourceRef{repo}, PinnedEndpoint: pin(repo), Options: o, Transfer: true, Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.client.valid(repo, "repository")
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[UploadSession], error) {
		raw, e := header(h, "Location", c.client.runtime.StructuredLimit())
		if e != nil {
			return core.Decoded[UploadSession]{}, e
		}
		out, e := c.uploadLocation(raw, repo, nil)
		decoded := core.Decoded[UploadSession]{Data: out}
		if out.Ref.ID != "" {
			decoded.Refs = []types.ResourceRef{out.Ref}
		}
		return decoded, e
	})
}
func (c *Uploads) Status(ctx context.Context, session UploadSession, o types.CallOptions) (types.Result[UploadSession], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.status")
	path, query, local := c.sessionTarget(session)
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, OCISessionQuery: query, Refs: []types.ResourceRef{session.Ref}, PinnedEndpoint: pin(session.Ref), Options: o, Transfer: true, Validate: func() error {
		if bind != nil {
			return bind
		}
		return local
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[UploadSession], error) {
		out, e := c.sessionResponse(h, session)
		return core.Decoded[UploadSession]{Data: out}, e
	})
}
func (c *Uploads) sessionTarget(session UploadSession) (string, string, error) {
	if e := c.client.valid(session.Ref, "upload"); e != nil {
		return "", "", e
	}
	saved, e := c.uploadLocation(string(session.Location.Reveal()), *session.Ref.Parent, &session.Ref)
	if e != nil {
		return "", "", e
	}
	target, _ := url.Parse(string(saved.Location.Reveal()))
	base, _ := url.Parse(session.Ref.EndpointURL)
	return strings.TrimPrefix(target.EscapedPath(), strings.TrimRight(base.EscapedPath(), "/")), target.RawQuery, nil
}
func (c *Uploads) sessionResponse(h http.Header, session UploadSession) (UploadSession, error) {
	raw, e := header(h, "Location", c.client.runtime.StructuredLimit())
	if e != nil {
		return UploadSession{}, e
	}
	out, e := c.uploadLocation(raw, *session.Ref.Parent, &session.Ref)
	if e != nil {
		return out, e
	}
	raw, e = header(h, "Range", c.client.runtime.StructuredLimit())
	if e != nil {
		return out, e
	}
	if !strings.HasPrefix(raw, "0-") {
		return out, types.NewError(types.ProtocolError)
	}
	digits := strings.TrimPrefix(raw, "0-")
	for _, v := range digits {
		if v < '0' || v > '9' {
			return out, types.NewError(types.ProtocolError)
		}
	}
	last, e := strconv.ParseUint(digits, 10, 64)
	if e != nil || last == ^uint64(0) {
		return out, types.NewError(types.ProtocolError)
	}
	out.Offset = last + 1
	return out, nil
}

type uploadReader struct {
	reader io.Reader
	read   int64
	ctx    context.Context
}

func (r *uploadReader) Read(p []byte) (int, error) {
	if r.ctx.Err() != nil {
		return 0, r.ctx.Err()
	}
	n, e := r.reader.Read(p)
	r.read += int64(n)
	return n, e
}
func (c *Uploads) Append(ctx context.Context, session UploadSession, body io.Reader, size int64, o types.CallOptions) (types.Result[UploadSession], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.append")
	path, query, local := c.sessionTarget(session)
	// The reader is consumed synchronously after the common preflight handshake.
	stream := &uploadReader{reader: io.LimitReader(body, size), ctx: ctx}
	op.Headers = http.Header{"Content-Type": {"application/octet-stream"}, "Content-Range": {strconv.FormatUint(session.Offset, 10) + "-" + strconv.FormatUint(session.Offset+uint64(size)-1, 10)}}
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, OCISessionQuery: query, Refs: []types.ResourceRef{session.Ref}, PinnedEndpoint: pin(session.Ref), Options: o, Transfer: true, BodyStream: stream, BodySize: size, Validate: func() error {
		if bind != nil {
			return bind
		}
		if local != nil {
			return local
		}
		if body == nil || size <= 0 || session.Offset > ^uint64(0)-uint64(size) {
			return types.NewError(types.InvalidArgument)
		}
		return nil
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[UploadSession], error) {
		out, e := c.sessionResponse(h, session)
		if e == nil && (stream.read != size || out.Offset != session.Offset+uint64(size)) {
			e = types.NewError(types.ProtocolError)
		}
		return core.Decoded[UploadSession]{Data: out}, e
	})
}

// Finalize completes the exact saved session. DigestVerified is true only when
// this call transmitted the whole blob and locally hashed every byte.
func (c *Uploads) Finalize(ctx context.Context, session UploadSession, digest Digest, body io.Reader, size int64, o types.CallOptions) (types.Result[BlobWriteResult], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.finalize")
	path, query, local := c.sessionTarget(session)
	requestedBlob := types.ResourceRef{}
	if session.Ref.Parent != nil {
		requestedBlob = child(*session.Ref.Parent, "blob", digest.String())
	}
	if query != "" {
		query += "&"
	}
	query += "digest=" + url.QueryEscape(digest.String())
	hashes := digestHashes()
	missingBody := body == nil && size > 0
	if body == nil {
		body = strings.NewReader("")
	}
	stream := &uploadReader{reader: io.TeeReader(io.LimitReader(body, size), io.MultiWriter(hashes["sha256"], hashes["sha384"], hashes["sha512"])), ctx: ctx}
	op.Headers = http.Header{"Content-Type": {"application/octet-stream"}}
	if size > 0 && session.Offset > 0 {
		op.Headers.Set("Content-Range", strconv.FormatUint(session.Offset, 10)+"-"+strconv.FormatUint(session.Offset+uint64(size)-1, 10))
	}
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, OCISessionQuery: query, Refs: []types.ResourceRef{session.Ref, requestedBlob}, PinnedEndpoint: pin(session.Ref), Options: o, Transfer: true, BodyStream: stream, BodySize: size, Validate: func() error {
		if bind != nil {
			return bind
		}
		if local != nil {
			return local
		}
		if missingBody || size < 0 || session.Offset > ^uint64(0)-uint64(size) {
			return types.NewError(types.InvalidArgument)
		}
		_, e := ParseDigest(digest.String())
		return e
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[BlobWriteResult], error) {
		out := BlobWriteResult{Digest: digest}
		location, e := header(h, "Location", c.client.runtime.StructuredLimit())
		if e != nil {
			return core.Decoded[BlobWriteResult]{Data: out}, e
		}
		base, _ := url.Parse(session.Ref.EndpointURL)
		target, e := url.Parse(location)
		if e != nil || location == "" || target.User != nil || target.Fragment != "" || target.Opaque != "" {
			return core.Decoded[BlobWriteResult]{Data: out}, types.NewError(types.ProtocolError)
		}
		requestURL, _ := url.Parse(string(session.Location.Reveal()))
		target = requestURL.ResolveReference(target)
		if !sameOrigin(base, target) {
			return core.Decoded[BlobWriteResult]{Data: out}, types.NewError(types.ProtocolError)
		}
		expected := strings.TrimRight(base.EscapedPath(), "/") + repositoryPath(*session.Ref.Parent) + "/blobs/" + url.PathEscape(digest.String())
		if target.EscapedPath() != expected {
			return core.Decoded[BlobWriteResult]{Data: out}, types.NewError(types.ScopeMismatch)
		}
		out.Address = child(*session.Ref.Parent, "blob", digest.String())
		out.Location = types.Protect([]byte(target.String()))
		decoded := func(e error) (core.Decoded[BlobWriteResult], error) {
			return core.Decoded[BlobWriteResult]{Data: out, Refs: []types.ResourceRef{out.Address}}, e
		}
		stated, e := header(h, "Docker-Content-Digest", c.client.runtime.StructuredLimit())
		if e != nil {
			return decoded(e)
		}
		if stated != "" && stated != digest.String() {
			return decoded(types.NewError(types.ProtocolError))
		}
		if stream.read != size {
			return decoded(types.NewError(types.ProtocolError))
		}
		if session.Offset == 0 {
			if sum(hashes[digest.Algorithm], digest.Algorithm) != digest {
				return decoded(types.NewError(types.ProtocolError))
			}
			out.DigestVerified = true
		}
		return decoded(nil)
	})
}

// CancelNative is the explicit pinned Distribution API cancellation variant.
// Its availability on the current Selectel service is not inferred from OCI.
func (c *Uploads) CancelNative(ctx context.Context, session UploadSession, o types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.cancel_native")
	path, query, local := c.sessionTarget(session)
	return core.DoResponse[struct{}](c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: path, OCISessionQuery: query, Refs: []types.ResourceRef{session.Ref}, PinnedEndpoint: pin(session.Ref), Options: o, Transfer: true, Validate: func() error {
		if bind != nil {
			return bind
		}
		return local
	}}, func(context.Context, io.Reader, types.EndpointBinding, int, http.Header) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}

// Monolithic explicitly attempts the optional OCI single-POST upload variant.
// A202 result returns a session only; the caller decides whether to continue.
// No automatic fallback consumes or retransmits the caller's reader.
func (c *Uploads) Monolithic(ctx context.Context, repo types.ResourceRef, digest Digest, body io.Reader, size int64, options types.CallOptions) (types.Result[MonolithicUploadResult], error) {
	op, bind := c.client.operation("registry_data.oci_v1.uploads.start")
	op.Success = []int{201, 202}
	op.Headers = http.Header{"Content-Type": {"application/octet-stream"}}
	requested := child(repo, "blob", digest.String())
	hashes := digestHashes()
	missing := body == nil && size > 0
	if body == nil {
		body = strings.NewReader("")
	}
	stream := &uploadReader{reader: io.TeeReader(io.LimitReader(body, size), io.MultiWriter(hashes["sha256"], hashes["sha384"], hashes["sha512"])), ctx: ctx}
	var requestBody io.Reader = stream
	if size == 0 {
		requestBody = nil
	}
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: repositoryPath(repo) + "/blobs/uploads/", Query: url.Values{"digest": {digest.String()}}, Refs: []types.ResourceRef{repo, requested}, PinnedEndpoint: pin(repo), Options: options, Transfer: true, BodyStream: requestBody, BodySize: size, Validate: func() error {
		if bind != nil {
			return bind
		}
		if e := c.client.valid(repo, "repository"); e != nil {
			return e
		}
		if missing || size < 0 {
			return types.NewError(types.InvalidArgument)
		}
		_, e := ParseDigest(digest.String())
		return e
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, status int, h http.Header) (core.Decoded[MonolithicUploadResult], error) {
		location, e := header(h, "Location", c.client.runtime.StructuredLimit())
		if e != nil {
			return core.Decoded[MonolithicUploadResult]{}, e
		}
		if status == 202 {
			session, e := c.uploadLocation(location, repo, nil)
			out := core.Decoded[MonolithicUploadResult]{}
			if session.Ref.ID != "" {
				out.Data.Session = &session
				out.Refs = []types.ResourceRef{session.Ref}
			}
			return out, e
		}
		base, _ := url.Parse(repo.EndpointURL)
		target, e := url.Parse(location)
		if e != nil || location == "" || target.User != nil || target.Fragment != "" || target.Opaque != "" {
			return core.Decoded[MonolithicUploadResult]{}, types.NewError(types.ProtocolError)
		}
		requestURL, _ := url.Parse(strings.TrimRight(repo.EndpointURL, "/") + repositoryPath(repo) + "/blobs/uploads/")
		target = requestURL.ResolveReference(target)
		if !sameOrigin(base, target) {
			return core.Decoded[MonolithicUploadResult]{}, types.NewError(types.ProtocolError)
		}
		if target.EscapedPath() != strings.TrimRight(base.EscapedPath(), "/")+repositoryPath(repo)+"/blobs/"+url.PathEscape(digest.String()) {
			return core.Decoded[MonolithicUploadResult]{}, types.NewError(types.ScopeMismatch)
		}
		blob := BlobWriteResult{Address: requested, Digest: digest, Location: types.Protect([]byte(target.String()))}
		out := core.Decoded[MonolithicUploadResult]{Data: MonolithicUploadResult{Blob: &blob}, Refs: []types.ResourceRef{requested}}
		stated, e := header(h, "Docker-Content-Digest", c.client.runtime.StructuredLimit())
		if e != nil {
			return out, e
		}
		if stated != "" && stated != digest.String() {
			return out, types.NewError(types.ProtocolError)
		}
		if stream.read != size || sum(hashes[digest.Algorithm], digest.Algorithm) != digest {
			return out, types.NewError(types.ProtocolError)
		}
		blob.DigestVerified = true
		return out, nil
	})
}
