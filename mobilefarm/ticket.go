package mobilefarm

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
)

// IssueTicket returns a single-delivery authorization value; it never opens a connection.
func (c *Client) IssueTicket(ctx context.Context, options types.CallOptions) (types.Result[GetTicketResponse], error) {
	op, bindErr := c.binding("mobile_farm.v2.api_auth_tickets.create", http.MethodPost, "mutation", projectOnly, []int{200}, nil, nil)
	out, e := core.DoJSON(c.runtime, ctx, op, core.Request{Scope: c.scope, Path: "/api/v2/auth/tickets", Options: options, Validate: func() error { return bindErr }}, func(raw []byte, _ types.EndpointBinding) (core.Decoded[GetTicketResponse], error) {
		var decoded core.Decoded[GetTicketResponse]
		if json.Unmarshal(raw, &decoded.Data) != nil || decoded.Data.Validate() != nil {
			return decoded, types.NewError(types.ProtocolError)
		}
		return decoded, nil
	})
	if out.Receipt != nil {
		out.Receipt.Recovery = "unavailable"
	}
	return out, e
}
