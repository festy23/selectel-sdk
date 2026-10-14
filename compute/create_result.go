package compute

import (
	"context"
	"encoding/json"
	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

// CreateResult preserves the provider's server-or-reservation response variant.
// A server response represents only the first VM of a multiple create.
type CreateResult struct {
	Server         *Server            `json:"server,omitempty"`
	ReservationID  *string            `json:"reservation_id,omitempty"`
	ReservationRef *types.ResourceRef `json:"reservation_ref,omitempty"`
}

func (s *Servers) CreateWithResult(ctx context.Context, r CreateRequest) (types.Result[CreateResult], error) {
	raw, err := createBody(r)
	op, versionErr := createOperation(r.Microversion)
	if err == nil {
		err = versionErr
	}
	reservation := r.ReturnReservationID != nil && *r.ReturnReservationID
	result, e := core.DoJSON(s.runtime, ctx, op, core.Request{Scope: s.scope, Path: "/servers", Body: raw, Validate: func() error { return err }}, func(raw []byte, endpoint types.EndpointBinding) (core.Decoded[CreateResult], error) {
		if !reservation {
			decoded, e := decodeCreatedServer(raw, endpoint)
			data := CreateResult{}
			if decoded.Data.Ref.ID != "" {
				data.Server = &decoded.Data
			}
			return core.Decoded[CreateResult]{Data: data, Refs: decoded.Refs}, e
		}
		var body struct {
			ReservationID *string         `json:"reservation_id"`
			Server        json.RawMessage `json:"server"`
		}
		if json.Unmarshal(raw, &body) != nil || body.ReservationID == nil || *body.ReservationID == "" {
			return core.Decoded[CreateResult]{}, types.NewError(types.ProtocolError)
		}
		reference := core.Ref(endpoint, "compute", "server_reservation", *body.ReservationID)
		decoded := core.Decoded[CreateResult]{Data: CreateResult{ReservationID: body.ReservationID, ReservationRef: reference}, Refs: []types.ResourceRef{*reference}}
		if len(body.Server) > 0 {
			return decoded, types.NewError(types.ProtocolError)
		}
		return decoded, nil
	})
	if reservation && result.Receipt != nil {
		result.Receipt.Recovery = "unavailable"
	}
	return result, e
}

func decodeCreatedServer(raw []byte, endpoint types.EndpointBinding) (core.Decoded[Server], error) {
	decoded, e := decodeOne("")(raw, endpoint)
	if e != nil {
		return decoded, e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return decoded, types.NewError(types.ProtocolError)
	}
	if _, present := fields["reservation_id"]; present {
		return decoded, types.NewError(types.ProtocolError)
	}
	return decoded, nil
}
