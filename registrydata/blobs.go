package registrydata

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"hash"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type Blobs struct{ client *Client }

func (c *Blobs) Digest(repo types.ResourceRef, digest Digest) (types.ResourceRef, error) {
	if e := c.client.valid(repo, "repository"); e != nil {
		return types.ResourceRef{}, e
	}
	if _, e := ParseDigest(digest.String()); e != nil {
		return types.ResourceRef{}, e
	}
	return child(repo, "blob", digest.String()), nil
}
func blobPath(ref types.ResourceRef) string {
	if ref.Parent == nil || ref.Parent.Parent == nil {
		return ""
	}
	return "/v2/" + ref.Parent.Parent.ID + "/" + ref.Parent.ID + "/blobs/" + url.PathEscape(ref.ID)
}
func integer(n uint64) types.Integer { v, _ := types.ParseInteger(strconv.FormatUint(n, 10)); return v }
func digestHashes() map[string]hash.Hash {
	return map[string]hash.Hash{"sha256": sha256.New(), "sha384": sha512.New384(), "sha512": sha512.New()}
}
func sum(h hash.Hash, algorithm string) Digest {
	return Digest{algorithm, hex.EncodeToString(h.Sum(nil))}
}
func hashChunk(h map[string]hash.Hash, raw []byte) {
	for _, v := range h {
		v.Write(raw)
	}
}
func rangeNumber(raw string) (*big.Int, error) {
	if raw == "" {
		return nil, types.NewError(types.ProtocolError)
	}
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return nil, types.NewError(types.ProtocolError)
		}
	}
	v, ok := new(big.Int).SetString(raw, 10)
	if !ok {
		return nil, types.NewError(types.ProtocolError)
	}
	return v, nil
}
func decodeRange(raw string) (ContentRange, error) {
	parts := strings.Fields(raw)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bytes") {
		return ContentRange{}, types.NewError(types.ProtocolError)
	}
	span, total, ok := strings.Cut(parts[1], "/")
	if !ok {
		return ContentRange{}, types.NewError(types.ProtocolError)
	}
	start, end, ok := strings.Cut(span, "-")
	if !ok {
		return ContentRange{}, types.NewError(types.ProtocolError)
	}
	a, e := rangeNumber(start)
	if e != nil {
		return ContentRange{}, e
	}
	b, e := rangeNumber(end)
	if e != nil || b.Cmp(a) < 0 {
		return ContentRange{}, types.NewError(types.ProtocolError)
	}
	out := ContentRange{}
	out.Start, _ = types.ParseInteger(a.String())
	out.End, _ = types.ParseInteger(b.String())
	if total != "*" {
		n, e := rangeNumber(total)
		if e != nil || n.Cmp(b) <= 0 {
			return ContentRange{}, types.NewError(types.ProtocolError)
		}
		v, _ := types.ParseInteger(n.String())
		out.Total = &v
	}
	return out, nil
}
func readPrefix(ctx context.Context, r io.Reader, n uint64, h map[string]hash.Hash) (uint64, error) {
	buf := make([]byte, 32*1024)
	var copied uint64
	empty := 0
	for copied < n {
		if ctx.Err() != nil {
			return copied, types.ContextError(ctx.Err())
		}
		size := uint64(len(buf))
		if n-copied < size {
			size = n - copied
		}
		count, e := r.Read(buf[:size])
		if count < 0 || count > int(size) {
			return copied, types.NewError(types.InvalidArgument)
		}
		if count > 0 {
			hashChunk(h, buf[:count])
			copied += uint64(count)
			empty = 0
		} else {
			empty++
		}
		if e != nil && copied < n {
			return copied, types.NewError(types.InvalidArgument)
		}
		if empty >= 100 {
			return copied, types.NewError(types.InvalidArgument)
		}
	}
	return copied, nil
}

func (c *Blobs) Download(ctx context.Context, ref types.ResourceRef, destination io.Writer, in BlobDownloadOptions, o types.CallOptions) (types.Result[BlobDownloadResult], error) {
	op, bind := c.client.operation("registry_data.oci_v1.blobs.get")
	op.Headers = http.Header{"Accept-Encoding": {"identity"}}
	if in.Offset > 0 {
		op.Headers.Set("Range", "bytes="+strconv.FormatUint(in.Offset, 10)+"-")
	}
	var expected Digest
	progress := BlobDownloadResult{}
	result, err := core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: blobPath(ref), Transfer: true, Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		if e := c.client.valid(ref, "blob"); e != nil {
			return e
		}
		if destination == nil || in.MaxBytes == 0 || in.Offset > in.MaxBytes || (in.Offset > 0 && in.Prefix == nil) || (in.Offset == 0 && in.Prefix != nil) {
			return types.NewError(types.InvalidArgument)
		}
		var e error
		expected, e = ParseDigest(ref.ID)
		return e
	}}, func(call context.Context, body io.Reader, _ types.EndpointBinding, status int, h http.Header) (decoded core.Decoded[BlobDownloadResult], failure error) {
		progress = BlobDownloadResult{Address: ref, ExpectedDigest: expected, BytesTransferred: integer(0), TotalBytes: integer(0)}
		hashes := digestHashes()
		var transferred, prefix uint64
		defer func() {
			actual := sum(hashes[expected.Algorithm], expected.Algorithm)
			progress.ActualDigest = &actual
			progress.BytesTransferred = integer(transferred)
			progress.TotalBytes = integer(prefix + transferred)
			decoded.Data = progress
		}()
		coding, e := header(h, "Content-Encoding", c.client.runtime.StructuredLimit())
		if e != nil {
			return decoded, e
		}
		if coding != "" && !strings.EqualFold(coding, "identity") {
			return decoded, types.NewError(types.ProtocolError)
		}
		stated, e := header(h, "Docker-Content-Digest", c.client.runtime.StructuredLimit())
		if e != nil {
			return decoded, e
		}
		if stated != "" {
			d, e := ParseDigest(stated)
			if e != nil {
				return decoded, types.NewError(types.ProtocolError)
			}
			progress.ProviderDigest = &d
		}
		var total, wireSize *big.Int
		rangeHeader, e := header(h, "Content-Range", c.client.runtime.StructuredLimit())
		if e != nil {
			return decoded, e
		}
		if in.Offset > 0 {
			if status != 206 {
				return decoded, types.NewError(types.ProtocolError)
			}
			r, e := decodeRange(rangeHeader)
			if e != nil {
				return decoded, e
			}
			start, _ := new(big.Int).SetString(r.Start.String(), 10)
			end, _ := new(big.Int).SetString(r.End.String(), 10)
			if start.Cmp(new(big.Int).SetUint64(in.Offset)) != 0 {
				return decoded, types.NewError(types.ProtocolError)
			}
			wireSize = new(big.Int).Add(new(big.Int).Sub(end, start), big.NewInt(1))
			progress.Range = &r
			if r.Total != nil {
				total, _ = new(big.Int).SetString(r.Total.String(), 10)
			}
		} else if status != 200 || rangeHeader != "" {
			return decoded, types.NewError(types.ProtocolError)
		}
		length, e := header(h, "Content-Length", c.client.runtime.StructuredLimit())
		if e != nil {
			return decoded, e
		}
		if length != "" {
			n, e := rangeNumber(length)
			if e != nil {
				return decoded, e
			}
			if wireSize != nil && n.Cmp(wireSize) != 0 {
				return decoded, types.NewError(types.ProtocolError)
			}
			wireSize = n
			if in.Offset == 0 {
				total = n
			}
		}
		max := new(big.Int).SetUint64(in.MaxBytes)
		if total != nil && total.Cmp(max) > 0 {
			return decoded, types.NewError(types.ResponseLimit)
		}
		if wireSize != nil && wireSize.Cmp(new(big.Int).SetUint64(in.MaxBytes-in.Offset)) > 0 {
			return decoded, types.NewError(types.ResponseLimit)
		}
		if in.Offset > 0 {
			var e error
			prefix, e = readPrefix(call, in.Prefix, in.Offset, hashes)
			if e != nil {
				return decoded, e
			}
		}
		buf := make([]byte, 32*1024)
		empty := 0
		for {
			if call.Err() != nil {
				return decoded, types.ContextError(call.Err())
			}
			remaining := in.MaxBytes - prefix - transferred
			size := uint64(len(buf))
			if remaining < size {
				size = remaining
			}
			if size == 0 {
				size = 1
			}
			n, readErr := body.Read(buf[:size])
			if n < 0 || n > int(size) {
				return decoded, types.NewError(types.ProtocolError)
			}
			if n > 0 {
				empty = 0
				if remaining == 0 {
					return decoded, types.NewError(types.ResponseLimit)
				}
				if wireSize != nil && new(big.Int).SetUint64(transferred+uint64(n)).Cmp(wireSize) > 0 {
					return decoded, types.NewError(types.ProtocolError)
				}
				written, writeErr := destination.Write(buf[:n])
				if written < 0 || written > n {
					return decoded, types.NewError(types.TransportError)
				}
				hashChunk(hashes, buf[:written])
				transferred += uint64(written)
				if writeErr != nil || written != n {
					return decoded, types.NewError(types.TransportError)
				}
			} else {
				empty++
			}
			if readErr != nil {
				if readErr != io.EOF {
					return decoded, types.NewError(types.TransportError)
				}
				break
			}
			if empty >= 100 {
				return decoded, types.NewError(types.TransportError)
			}
		}
		if wireSize != nil && new(big.Int).SetUint64(transferred).Cmp(wireSize) != 0 {
			return decoded, types.NewError(types.ProtocolError)
		}
		if total != nil && new(big.Int).SetUint64(prefix+transferred).Cmp(total) < 0 {
			return decoded, types.NewError(types.Incomplete)
		}
		actual := sum(hashes[expected.Algorithm], expected.Algorithm)
		if actual != expected {
			return decoded, types.NewError(types.ProtocolError)
		}
		if progress.ProviderDigest != nil {
			d := *progress.ProviderDigest
			if sum(hashes[d.Algorithm], d.Algorithm) != d {
				return decoded, types.NewError(types.ProtocolError)
			}
		}
		progress.DigestVerified = true
		progress.Complete = true
		return decoded, nil
	})
	if err != nil {
		result.Data = progress
	}
	return result, err
}

func (c *Blobs) valid(ref types.ResourceRef) error {
	if e := c.client.valid(ref, "blob"); e != nil {
		return e
	}
	_, e := ParseDigest(ref.ID)
	return e
}
func (c *Blobs) Head(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[BlobInfo], error) {
	op, bind := c.client.operation("registry_data.oci_v1.blobs.head")
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: blobPath(ref), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.valid(ref)
	}}, func(_ context.Context, _ io.Reader, _ types.EndpointBinding, _ int, h http.Header) (core.Decoded[BlobInfo], error) {
		v, e := contentInfo(h, ref, c.client.runtime.StructuredLimit())
		return core.Decoded[BlobInfo]{Data: v}, e
	})
}
func (c *Blobs) Delete(ctx context.Context, ref types.ResourceRef, o types.CallOptions) (types.Result[struct{}], error) {
	op, bind := c.client.operation("registry_data.oci_v1.blobs.delete")
	return core.DoResponse(c.client.runtime, ctx, op, core.Request{Scope: c.client.scope, Path: blobPath(ref), Options: o, Refs: []types.ResourceRef{ref}, PinnedEndpoint: pin(ref), Validate: func() error {
		if bind != nil {
			return bind
		}
		return c.valid(ref)
	}}, func(context.Context, io.Reader, types.EndpointBinding, int, http.Header) (core.Decoded[struct{}], error) {
		return core.Decoded[struct{}]{}, nil
	})
}
