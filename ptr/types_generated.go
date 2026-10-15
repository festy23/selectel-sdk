// Code generated from PTR OpenAPI SHA256 e8043681a667a1978f5b7966cffdc9300d485a20e6474a3da20c9d52d4b75aba; DO NOT EDIT.
package ptr

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
)

type Record struct {
	ID      types.Integer        `json:"id"`
	Content types.ProtectedValue `json:"content"`
	IP      string               `json:"ip"`
	UserID  types.Integer        `json:"user_id"`
	TTL     types.Integer        `json:"ttl"`
	Ref     *types.ResourceRef   `json:"ref,omitempty"`
}
type CreateRequest struct {
	IP      string                `json:"ip"`
	Content *types.ProtectedValue `json:"content,omitempty"`
	TTL     *types.Integer        `json:"ttl,omitempty"`
}
type UpdateRequest struct {
	IP      *string               `json:"ip,omitempty"`
	Content *types.ProtectedValue `json:"content,omitempty"`
}

func (v *Record) UnmarshalJSON(raw []byte) error {
	var x struct {
		ID      *types.Integer `json:"id"`
		Content *string        `json:"content"`
		IP      *string        `json:"ip"`
		UserID  *types.Integer `json:"user_id"`
		TTL     *types.Integer `json:"ttl"`
	}
	if json.Unmarshal(raw, &x) != nil || x.ID == nil || x.Content == nil || x.IP == nil || x.UserID == nil || x.TTL == nil {
		return types.NewError(types.ProtocolError)
	}
	v.ID = *x.ID
	v.Content = types.Protect([]byte(*x.Content))
	v.IP = *x.IP
	v.UserID = *x.UserID
	v.TTL = *x.TTL
	return nil
}
