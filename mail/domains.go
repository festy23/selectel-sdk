package mail

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/url"
	"unicode/utf8"
)

func decode[T any](raw []byte, _ types.EndpointBinding) (core.Decoded[T], error) {
	out := core.Decoded[T]{}
	if json.Unmarshal(raw, &out.Data) != nil {
		return out, types.NewError(types.ProtocolError)
	}
	if v, ok := any(out.Data).(interface{ Validate() error }); ok {
		return out, v.Validate()
	}
	return out, nil
}
func checkedQuery(req *core.Request, values map[string]*types.ProtectedValue) {
	validate := req.Validate
	var valueErr error
	req.Query = url.Values{}
	for key, v := range values {
		if v == nil {
			continue
		}
		if !utf8.Valid(v.Reveal()) {
			valueErr = types.NewError(types.InvalidArgument)
			continue
		}
		req.Query.Set(key, string(v.Reveal()))
	}
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return valueErr
	}
}
func (c *Client) LinkDomain(ctx context.Context, ref types.ResourceRef, spec CreateDomainRequest, o types.CallOptions) (types.Result[CreateDomain], error) {
	op, err := c.operation("linkDomain")
	req := c.request(ref, err)
	req.Path += "/domains"
	req.Options = o
	body, bodyErr := protectedBody("name", spec.Name, spec.Extra)
	req.Body = body
	validate := req.Validate
	req.Validate = func() error {
		if e := validate(); e != nil {
			return e
		}
		return bodyErr
	}
	return core.DoJSON(c.runtime, ctx, op, req, decode[CreateDomain])
}
func (c *Client) CheckDomain(ctx context.Context, ref types.ResourceRef, name types.ProtectedValue, o types.CallOptions) (types.Result[DomainVerification], error) {
	op, err := c.operation("checkDomainVerificationStatus")
	req := c.request(ref, err)
	req.Path += "/domains/check"
	req.Options = o
	checkedQuery(&req, map[string]*types.ProtectedValue{"name": &name})
	return core.DoJSON(c.runtime, ctx, op, req, decode[DomainVerification])
}
func (c *Client) UnlinkDomain(ctx context.Context, ref types.ResourceRef, name types.ProtectedValue, o types.CallOptions) (types.Result[struct{}], error) {
	op, err := c.operation("unlinkDomain")
	op.NoContentSuccess = []int{204}
	req := c.request(ref, err)
	req.Path += "/domains"
	req.Options = o
	checkedQuery(&req, map[string]*types.ProtectedValue{"name": &name})
	return core.DoJSON(c.runtime, ctx, op, req, emptyDecode)
}
