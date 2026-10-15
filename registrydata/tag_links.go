package registrydata

import (
	"github.com/festy23/selectel-sdk/types"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// tagNext accepts RFC5988 syntax while retaining the bounded n/last contract.
// Unselected relations are never followed; link parameters cannot grant trust.
func tagNext(h http.Header, repo types.ResourceRef, path, previous string, size uint, rows int, limit uint64) (bool, error) {
	values := h.Values("Link")
	var length uint64
	for i, value := range values {
		length += uint64(len(value))
		if i != 0 {
			length++
		}
		if length > limit {
			return false, types.NewError(types.ResponseLimit)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return false, types.NewError(types.ProtocolError)
		}
	}
	links, e := splitReferrerLinks(strings.Join(values, ","))
	if e != nil {
		return false, e
	}
	found := false
	for _, link := range links {
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}
		end := strings.IndexByte(link, '>')
		if !strings.HasPrefix(link, "<") || end < 1 {
			return false, types.NewError(types.ProtocolError)
		}
		_, params, e := mime.ParseMediaType("application/octet-stream" + link[end+1:])
		if e != nil {
			return false, types.NewError(types.ProtocolError)
		}
		selected := false
		for _, relation := range strings.Fields(params["rel"]) {
			if strings.EqualFold(relation, "next") {
				selected = true
			}
		}
		if !selected {
			continue
		}
		_, anchor := params["anchor"]
		if found || anchor || rows == 0 {
			return false, types.NewError(types.ProtocolError)
		}
		u, e := url.Parse(link[1:end])
		if e != nil || u.User != nil || u.Fragment != "" || u.Opaque != "" {
			return false, types.NewError(types.ProtocolError)
		}
		base, _ := url.Parse(repo.EndpointURL)
		requestURL, _ := url.Parse(strings.TrimRight(repo.EndpointURL, "/") + path)
		u = requestURL.ResolveReference(u)
		q, e := url.ParseQuery(u.RawQuery)
		if e != nil || !sameOrigin(base, u) || u.EscapedPath() != strings.TrimRight(base.EscapedPath(), "/")+path || len(q["last"]) != 1 || q.Get("last") != previous {
			return false, types.NewError(types.ScopeMismatch)
		}
		// n is source-optional with last; the SDK always sends its bounded page size.
		for name := range q {
			if name != "last" && name != "n" {
				return false, types.NewError(types.ScopeMismatch)
			}
		}
		if n, ok := q["n"]; ok && (len(n) != 1 || n[0] != strconv.FormatUint(uint64(size), 10)) {
			return false, types.NewError(types.ScopeMismatch)
		}
		found = true
	}
	return found, nil
}
