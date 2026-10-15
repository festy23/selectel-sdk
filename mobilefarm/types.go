package mobilefarm

import "github.com/festy23/selectel-sdk/types"

// NoContent represents an operation whose documented success has no body.
type NoContent struct{}

// UnmodeledResponse preserves confirmed HTTP acceptance without claiming a
// response schema, completion, or ownership association absent from the source.
// Body is bounded by the shared runtime and revealed only by the caller.
type UnmodeledResponse struct {
	HTTPStatus int                  `json:"http_status"`
	Body       types.ProtectedValue `json:"body"`
}
