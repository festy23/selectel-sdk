package s3

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/xml"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"io"
	"net/http"
	"net/url"
	"unicode/utf8"
)

func (c *Objects) DeleteMany(ctx context.Context, bucket types.ResourceRef, in DeleteManyRequest, o types.CallOptions) (types.Result[DeleteManyResponse], error) {
	s := c.client
	type entry struct {
		Key       string  `xml:"Key"`
		VersionID *string `xml:"VersionId,omitempty"`
	}
	body := struct {
		XMLName xml.Name `xml:"Delete"`
		XMLNS   string   `xml:"xmlns,attr"`
		Objects []entry  `xml:"Object"`
		Quiet   *bool    `xml:"Quiet,omitempty"`
	}{XMLNS: "http://s3.amazonaws.com/doc/2006-03-01/", Quiet: in.Quiet}
	for _, v := range in.Objects {
		x := entry{Key: v.Key}
		if v.VersionID != nil {
			version := string(v.VersionID.Reveal())
			x.VersionID = &version
		}
		body.Objects = append(body.Objects, x)
	}
	raw, encode := xml.Marshal(body)
	sum := md5.Sum(raw)
	h := http.Header{"Content-Type": {"application/xml"}, "Content-MD5": {base64.StdEncoding.EncodeToString(sum[:])}}
	return invoke(s, ctx, "objects.delete_many", "POST", bucket, url.Values{"delete": {""}}, h, raw, []int{200}, o, func() error {
		if encode != nil || len(in.Objects) == 0 || len(in.Objects) > 1000 {
			return types.NewError(types.InvalidArgument)
		}
		for _, v := range in.Objects {
			if !xmlText(v.Key) || (v.VersionID != nil && !xmlText(string(v.VersionID.Reveal()))) {
				return types.NewError(types.InvalidArgument)
			}
			if _, e := s.Object(bucket, v.Key); e != nil {
				return e
			}
		}
		return nil
	}, func(_ context.Context, b io.Reader, _ types.EndpointBinding, _ int, _ http.Header) (core.Decoded[DeleteManyResponse], error) {
		var body struct {
			XMLName xml.Name `xml:"DeleteResult"`
			Deleted []struct {
				Key                   string  `xml:"Key"`
				VersionID             *string `xml:"VersionId"`
				DeleteMarker          *bool   `xml:"DeleteMarker"`
				DeleteMarkerVersionID *string `xml:"DeleteMarkerVersionId"`
			} `xml:"Deleted"`
			Errors []struct {
				Key       string  `xml:"Key"`
				VersionID *string `xml:"VersionId"`
				Code      string  `xml:"Code"`
			} `xml:"Error"`
		}
		out := DeleteManyResponse{}
		if e := s.readXML(b, &body); e != nil {
			return core.Decoded[DeleteManyResponse]{Data: out}, e
		}
		known := map[string]bool{}
		currentKeys := map[string]bool{}
		for _, v := range in.Objects {
			version := ""
			if v.VersionID != nil {
				version = string(v.VersionID.Reveal())
			} else {
				currentKeys[v.Key] = true
			}
			known[v.Key+"\x00"+version] = true
		}
		seen := map[string]bool{}
		check := func(key string, version *string) error {
			v := ""
			if version != nil {
				v = *version
			}
			if !currentKeys[key] && !known[key+"\x00"+v] {
				return types.NewError(types.ScopeMismatch)
			}
			identity := key + "\x00" + v
			if currentKeys[key] && !known[identity] {
				identity = key + "\x00"
			}
			if seen[identity] {
				return types.NewError(types.ProtocolError)
			}
			seen[identity] = true
			return nil
		}
		for _, v := range body.Deleted {
			if e := check(v.Key, v.VersionID); e != nil {
				return core.Decoded[DeleteManyResponse]{Data: out}, e
			}
			item := DeleteResult{Key: v.Key, DeleteMarker: v.DeleteMarker}
			if v.VersionID != nil {
				p := types.Protect([]byte(*v.VersionID))
				item.VersionID = &p
			}
			if v.DeleteMarkerVersionID != nil {
				p := types.Protect([]byte(*v.DeleteMarkerVersionID))
				item.DeleteMarkerVersionID = &p
			}
			out.Deleted = append(out.Deleted, item)
		}
		for _, v := range body.Errors {
			if e := check(v.Key, v.VersionID); e != nil {
				return core.Decoded[DeleteManyResponse]{Data: out}, e
			}
			item := DeleteFailure{Key: v.Key, Code: v.Code}
			if v.VersionID != nil {
				p := types.Protect([]byte(*v.VersionID))
				item.VersionID = &p
			}
			out.Errors = append(out.Errors, item)
		}
		if (in.Quiet == nil || !*in.Quiet) && len(seen) != len(known) {
			return core.Decoded[DeleteManyResponse]{Data: out}, types.NewError(types.ProtocolError)
		}
		out.Complete = len(out.Errors) == 0
		if !out.Complete {
			e := types.NewError(types.ProviderError)
			e.Reason = "partial_success"
			return core.Decoded[DeleteManyResponse]{Data: out}, e
		}
		return core.Decoded[DeleteManyResponse]{Data: out}, nil
	})
}

func xmlText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if !(r == 0x9 || r == 0xA || r == 0xD || r >= 0x20 && r <= 0xD7FF || r >= 0xE000 && r <= 0xFFFD || r >= 0x10000 && r <= 0x10FFFF) {
			return false
		}
	}
	return true
}
