package globalrouter

import (
	"net/http"
	"regexp"

	"github.com/festy23/selectel-sdk/types"
)

type HeaderOptions struct{ AcceptLanguage, RequestID string }

var uuidV4 = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-4[0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

// WithHeaders returns a new view; the receiver and its scoped methods stay immutable.
func (c *Client) WithHeaders(o HeaderOptions) (*Client, error) {
	if o.AcceptLanguage != "" && o.AcceptLanguage != "en" && o.AcceptLanguage != "ru" {
		return nil, types.NewError(types.InvalidArgument)
	}
	if o.RequestID != "" && !uuidV4.MatchString(o.RequestID) {
		return nil, types.NewError(types.InvalidArgument)
	}
	view := Bind(c.runtime, c.scope)
	view.headers = http.Header{}
	if o.AcceptLanguage != "" {
		view.headers.Set("Accept-Language", o.AcceptLanguage)
	}
	if o.RequestID != "" {
		view.headers.Set("X-Request-ID", o.RequestID)
	}
	return view, nil
}
