// Package swift implements the confirmed current Selectel Swift protocol.
package swift

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

type Client struct {
	runtime *core.Runtime
	scope   types.Scope
}

func Bind(r *core.Runtime, scope types.Scope) *Client { return &Client{r, scope} }
func (c *Client) operation(id, method string, success []int) (core.Operation, error) {
	effect := "mutation"
	if method == "HEAD" || method == "GET" {
		effect = "read"
	}
	return c.runtime.BindDocumented(core.Operation{ID: id, Service: "swift", APIGeneration: "v1", Method: method, Effect: effect, Auth: types.IAMProject, RequireProject: true, RequireLocation: true, Success: success, Rejected: []int{400, 401, 403, 404, 409, 412, 413, 422, 429}}, c.scope)
}
func integer(header http.Header, key string) (*types.Integer, error) {
	values := header.Values(key)
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, types.NewError(types.ProtocolError)
	}
	n, e := types.ParseInteger(values[0])
	if e != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	return &n, nil
}
func number(header http.Header, key string) (*types.Number, error) {
	values := header.Values(key)
	if len(values) == 0 {
		return nil, nil
	}
	if len(values) != 1 {
		return nil, types.NewError(types.ProtocolError)
	}
	n, e := types.ParseNumber(values[0])
	if e != nil {
		return nil, types.NewError(types.ProtocolError)
	}
	return &n, nil
}
func metadata(header http.Header, prefix string) map[string]types.ProtectedValue {
	out := map[string]types.ProtectedValue{}
	for k, values := range header {
		if strings.HasPrefix(strings.ToLower(k), strings.ToLower(prefix)) {
			out[k[len(prefix):]] = types.Protect([]byte(strings.Join(values, ",")))
		}
	}
	return out
}
func (c *Client) Account(ctx context.Context, o types.CallOptions) (types.Result[AccountInfo], error) {
	return invoke(c, ctx, "swift.v1.account.get", "HEAD", accountPath(c.scope), []int{204}, nil, nil, nil, nil, o, nil, func(_ context.Context, _ io.Reader, ep types.EndpointBinding, _ int, h http.Header) (core.Decoded[AccountInfo], error) {
		out, e := accountInfo(h, accountRef(ep))
		return core.Decoded[AccountInfo]{Data: out}, e
	})
}
