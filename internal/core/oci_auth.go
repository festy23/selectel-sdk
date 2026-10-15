package core

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

// ociScope accepts compiled OCI bindings only and derives access from the exact
// addressed repository, never from the server-advertised challenge scope.
func ociScope(op Operation, in Request, endpoint types.EndpointBinding) (string, error) {
	if op.ID == "registry_data.oci_v1.version.get" && op.Service == "registry_data" && op.APIGeneration == "oci_v1" && op.Auth == types.Basic && op.Method == "GET" && op.Effect == "read" && len(in.Refs) == 0 && in.Path == "/v2/" {
		return "", nil
	}
	if op.Service != "registry_data" || op.APIGeneration != "oci_v1" || op.Auth != types.Basic || ((op.ID == "registry_data.oci_v1.uploads.finalize" && len(in.Refs) != 2) || (op.ID != "registry_data.oci_v1.uploads.finalize" && len(in.Refs) != 1)) {
		return "", types.NewError(types.InvalidArgument)
	}
	ref := in.Refs[0]
	if ref.Parent == nil || (ref.Parent.Parent == nil && op.ID != "registry_data.oci_v1.uploads.start" && op.ID != "registry_data.oci_v1.tags.list") {
		return "", types.NewError(types.InvalidArgument)
	}
	repo, parent := ref.Parent, ref.Parent.Parent
	if op.ID == "registry_data.oci_v1.uploads.start" || op.ID == "registry_data.oci_v1.tags.list" {
		repo = &ref
		parent = ref.Parent
	}
	if repo.Kind != "repository" || parent.Kind != "registry_namespace" || parent.Parent != nil {
		return "", types.NewError(types.InvalidArgument)
	}
	for _, r := range []types.ResourceRef{ref, *repo, *parent} {
		if r.Scope != in.Scope || r.Service != op.Service || r.APIGeneration != op.APIGeneration || r.EndpointID != endpoint.ID || r.EndpointURL != endpoint.URL {
			return "", types.NewError(types.ScopeMismatch)
		}
	}
	if len(in.Refs) == 2 {
		extra := in.Refs[1]
		query, e := url.ParseQuery(in.OCISessionQuery)
		if e != nil || extra.Kind != "blob" || extra.ID != query.Get("digest") || extra.Parent == nil || !reflect.DeepEqual(*extra.Parent, *repo) {
			return "", types.NewError(types.InvalidArgument)
		}
		if extra.Scope != in.Scope || extra.Service != op.Service || extra.APIGeneration != op.APIGeneration || extra.EndpointID != endpoint.ID || extra.EndpointURL != endpoint.URL {
			return "", types.NewError(types.ScopeMismatch)
		}
	}
	namespace := parent.ID + "/" + repo.ID
	if !ociName.MatchString(namespace) {
		return "", types.NewError(types.InvalidArgument)
	}
	expectedMethod, action, effect := "", "", ""
	switch op.ID {
	case "registry_data.oci_v1.tags.list":
		if op.Method != "GET" || op.Effect != "read" || ref.Kind != "repository" || in.Path != "/v2/"+namespace+"/tags/list" {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull", nil
	case "registry_data.oci_v1.uploads.start":
		if op.Method != "POST" || op.Effect != "mutation" || ref.Kind != "repository" || in.Path != "/v2/"+namespace+"/blobs/uploads/" {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull,push", nil
	case "registry_data.oci_v1.uploads.cancel_native":
		if op.Method != "DELETE" || op.Effect != "mutation" || ref.Kind != "upload" || in.Path != "/v2/"+namespace+"/blobs/uploads/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":delete", nil
	case "registry_data.oci_v1.uploads.finalize":
		if op.Method != "PUT" || op.Effect != "mutation" || ref.Kind != "upload" || in.Path != "/v2/"+namespace+"/blobs/uploads/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull,push", nil
	case "registry_data.oci_v1.uploads.append":
		if op.Method != "PATCH" || op.Effect != "mutation" || ref.Kind != "upload" || in.Path != "/v2/"+namespace+"/blobs/uploads/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull,push", nil
	case "registry_data.oci_v1.uploads.status":
		if op.Method != "GET" || op.Effect != "read" || ref.Kind != "upload" || in.Path != "/v2/"+namespace+"/blobs/uploads/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull", nil
	case "registry_data.oci_v1.blobs.head", "registry_data.oci_v1.blobs.delete":
		method, action, effect := "HEAD", "pull", "read"
		if op.ID == "registry_data.oci_v1.blobs.delete" {
			method, action, effect = "DELETE", "delete", "mutation"
		}
		if op.Method != method || op.Effect != effect || ref.Kind != "blob" || in.Path != "/v2/"+namespace+"/blobs/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":" + action, nil
	case "registry_data.oci_v1.blobs.get":
		if op.Method != "GET" || op.Effect != "read" || ref.Kind != "blob" || in.Path != "/v2/"+namespace+"/blobs/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":pull", nil
	case "registry_data.oci_v1.manifests.get":
		expectedMethod, action, effect = "GET", "pull", "read"
	case "registry_data.oci_v1.manifests.head":
		expectedMethod, action, effect = "HEAD", "pull", "read"
	case "registry_data.oci_v1.manifests.replace":
		expectedMethod, action, effect = "PUT", "pull,push", "mutation"
	case "registry_data.oci_v1.manifests.delete":
		expectedMethod, action, effect = "DELETE", "delete", "mutation"
	}
	if expectedMethod != "" {
		if op.Method != expectedMethod || op.Effect != effect || (ref.Kind != "manifest_tag" && ref.Kind != "manifest_digest") || in.Path != "/v2/"+namespace+"/manifests/"+url.PathEscape(ref.ID) {
			return "", types.NewError(types.InvalidArgument)
		}
		return "repository:" + namespace + ":" + action, nil
	}

	return "", types.NewError(types.CapabilityUnavailable)
}

var ociName = regexp.MustCompile(`^[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*(\/[a-z0-9]+((\.|_|__|-+)[a-z0-9]+)*)*$`)
var bearerValue = regexp.MustCompile(`^[A-Za-z0-9._~+/-]+=*$`)

func (r *Runtime) authorizeOCI(ctx context.Context, entry *authEntry, op Operation, in Request, endpoint types.EndpointBinding) (*authEntry, error) {
	scope, e := ociScope(op, in, endpoint)
	if e != nil {
		return nil, e
	}
	if entry.material.Scheme != credentials.Basic {
		return nil, types.NewError(types.CredentialsUnavailable)
	}
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	probe, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint.URL, "/")+"/v2/", nil)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	resp, e := r.http.Do(probe)
	if e != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, types.ContextError(ctx.Err())
		}
		return nil, types.NewError(types.TransportError)
	}
	if resp == nil || resp.Body == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	resp.Body.Close()
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	copy := *entry
	if resp.StatusCode == 200 {
		return &copy, nil
	}
	if resp.StatusCode != 401 {
		return nil, ociAuthStatus(resp.StatusCode)
	}
	values := resp.Header.Values("WWW-Authenticate")
	var selected map[string]string
	basic := false
	var length uint64
	for _, value := range values {
		length += uint64(len(value))
		if length > r.config.MaxJSONBytes {
			return nil, types.NewError(types.ResponseLimit)
		}
		scheme, params, e := ociChallenge(value)
		if e != nil {
			return nil, e
		}
		switch strings.ToLower(scheme) {
		case "bearer":
			if selected != nil {
				return nil, types.NewError(types.ProtocolError)
			}
			selected = params
		case "basic":
			if params["realm"] == "" {
				return nil, types.NewError(types.ProtocolError)
			}
			basic = true
		}
	}
	if selected == nil {
		if basic {
			return &copy, nil
		}
		return nil, types.NewError(types.Unauthenticated)
	}
	realm, e := url.Parse(selected["realm"])
	base, _ := url.Parse(endpoint.URL)
	if e != nil || realm == nil || realm.Scheme != "https" || realm.Opaque != "" || realm.User != nil || realm.Fragment != "" || realm.Host == "" || !sameOCIOrigin(realm, base) {
		return nil, types.NewError(types.ProtocolError)
	}
	service := selected["service"]
	if service == "" || len(service) > 1024 || strings.IndexFunc(service, unicode.IsControl) >= 0 {
		return nil, types.NewError(types.ProtocolError)
	}
	query, e := url.ParseQuery(realm.RawQuery)
	if e != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	for key := range query {
		if key != "service" && key != "scope" {
			return nil, types.NewError(types.ProtocolError)
		}
	}
	query.Del("service")
	if service != "" {
		query.Set("service", service)
	}
	query.Del("scope")
	if scope != "" {
		query.Set("scope", scope)
	}
	realm.RawQuery = query.Encode()
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	request, e := http.NewRequestWithContext(ctx, "GET", realm.String(), nil)
	if e != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	request.SetBasicAuth(string(entry.material.Username.Reveal()), string(entry.material.Password.Reveal()))
	tokenResponse, e := r.http.Do(request)
	if e != nil {
		if tokenResponse != nil && tokenResponse.Body != nil {
			tokenResponse.Body.Close()
		}
		if ctx.Err() != nil {
			return nil, types.ContextError(ctx.Err())
		}
		return nil, types.NewError(types.TransportError)
	}
	if tokenResponse == nil || tokenResponse.Body == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	closeBody := sync.OnceFunc(func() { tokenResponse.Body.Close() })
	stop := context.AfterFunc(ctx, closeBody)
	defer func() { stop(); closeBody() }()
	if tokenResponse.StatusCode != 200 {
		return nil, ociAuthStatus(tokenResponse.StatusCode)
	}
	raw, e := readResponse(tokenResponse.Body, r.config.MaxJSONBytes)
	if ctx.Err() != nil {
		return nil, types.ContextError(ctx.Err())
	}
	if e != nil {
		return nil, e
	}
	var token struct {
		Token       *string `json:"token"`
		AccessToken *string `json:"access_token"`
	}
	if json.Unmarshal(raw, &token) != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	value := ""
	if token.Token != nil {
		value = *token.Token
	}
	if token.AccessToken != nil {
		if value != "" && value != *token.AccessToken {
			return nil, types.NewError(types.ProtocolError)
		}
		value = *token.AccessToken
	}
	if !bearerValue.MatchString(value) {
		return nil, types.NewError(types.ProtocolError)
	}
	copy.token = types.Protect([]byte(value))
	return &copy, nil
}
func sameOCIOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if u.Port() == "" {
			return "443"
		}
		return u.Port()
	}
	return strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// ociChallenge parses one source challenge with quoted-string escapes. Ambiguous
// combined/duplicate challenges fail closed rather than selecting a foreign URL.
func ociChallenge(raw string) (string, map[string]string, error) {
	raw = strings.TrimSpace(raw)
	cut := strings.IndexAny(raw, " \t")
	if cut < 1 {
		return "", nil, types.NewError(types.ProtocolError)
	}
	scheme, rest := raw[:cut], strings.TrimSpace(raw[cut:])
	params := map[string]string{}
	for rest != "" {
		if len(params) >= 32 {
			return "", nil, types.NewError(types.ResponseLimit)
		}
		i := 0
		for i < len(rest) && ociTokenByte(rest[i]) {
			i++
		}
		if i == 0 {
			return "", nil, types.NewError(types.ProtocolError)
		}
		key := strings.ToLower(rest[:i])
		rest = strings.TrimSpace(rest[i:])
		if !strings.HasPrefix(rest, "=") {
			return "", nil, types.NewError(types.ProtocolError)
		}
		rest = strings.TrimSpace(rest[1:])
		if _, ok := params[key]; ok {
			return "", nil, types.NewError(types.ProtocolError)
		}
		value := ""
		if strings.HasPrefix(rest, "\"") {
			rest = rest[1:]
			var out strings.Builder
			closed := false
			for len(rest) > 0 {
				ch := rest[0]
				rest = rest[1:]
				if ch == '"' {
					closed = true
					break
				}
				if ch == '\\' {
					if len(rest) == 0 {
						return "", nil, types.NewError(types.ProtocolError)
					}
					ch = rest[0]
					rest = rest[1:]
				}
				if ch < 32 || ch == 127 {
					return "", nil, types.NewError(types.ProtocolError)
				}
				out.WriteByte(ch)
			}
			if !closed {
				return "", nil, types.NewError(types.ProtocolError)
			}
			value = out.String()
		} else {
			i = 0
			for i < len(rest) && ociTokenByte(rest[i]) {
				i++
			}
			if i == 0 {
				return "", nil, types.NewError(types.ProtocolError)
			}
			value = rest[:i]
			rest = rest[i:]
		}
		params[key] = value
		rest = strings.TrimSpace(rest)
		if rest != "" {
			if rest[0] != ',' {
				return "", nil, types.NewError(types.ProtocolError)
			}
			rest = strings.TrimSpace(rest[1:])
			if rest == "" {
				return "", nil, types.NewError(types.ProtocolError)
			}
		}
	}
	return scheme, params, nil
}
func ociTokenByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))
}

func ociProviderCode(raw []byte) string {
	var v struct {
		Errors []struct {
			Code string `json:"code"`
		} `json:"errors"`
	}
	if json.Unmarshal(raw, &v) != nil || len(v.Errors) == 0 {
		return ""
	}
	switch v.Errors[0].Code {
	case "BLOB_UNKNOWN", "BLOB_UPLOAD_INVALID", "BLOB_UPLOAD_UNKNOWN", "DIGEST_INVALID", "MANIFEST_BLOB_UNKNOWN", "MANIFEST_INVALID", "MANIFEST_UNKNOWN", "NAME_INVALID", "NAME_UNKNOWN", "SIZE_INVALID", "UNAUTHORIZED", "DENIED", "UNSUPPORTED":
		return v.Errors[0].Code
	}
	return ""
}

func ociAuthStatus(status int) *types.SDKError {
	e := statusError(status)
	if status == 404 {
		e = types.NewError(types.EndpointUnavailable)
		e.HTTPStatus = status
	}
	return e
}
