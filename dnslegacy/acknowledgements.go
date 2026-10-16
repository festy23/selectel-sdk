package dnslegacy

import (
	"bytes"
	"encoding/json"

	"github.com/festy23/selectel-sdk/internal/core"
	"github.com/festy23/selectel-sdk/types"
)

func acknowledgement(raw []byte, _ types.EndpointBinding) (core.Decoded[Acknowledgement], error) {
	ack := Acknowledgement{}
	if len(bytes.TrimSpace(raw)) > 0 {
		var value types.ProtectedJSON
		if json.Unmarshal(raw, &value) != nil {
			return core.Decoded[Acknowledgement]{}, types.NewError(types.ProtocolError)
		}
		ack.Representation = &value
	}
	return core.Decoded[Acknowledgement]{Data: ack}, nil
}
