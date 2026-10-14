package resell

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"time"
)

// TrafficTimestamp preserves a timezone-free provider timestamp without guessing
// a location. UTC is available only when the provider included an offset.
type TrafficTimestamp struct {
	Raw string     `json:"raw"`
	UTC *time.Time `json:"utc,omitempty"`
}

func (t *TrafficTimestamp) UnmarshalJSON(raw []byte) error {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return types.NewError(types.ProtocolError)
	}
	if instant, e := time.Parse(time.RFC3339Nano, value); e == nil {
		utc := instant.UTC()
		*t = TrafficTimestamp{Raw: value, UTC: &utc}
		return nil
	}
	if _, e := time.Parse("2006-01-02T15:04:05", value); e != nil {
		return types.NewError(types.ProtocolError)
	}
	*t = TrafficTimestamp{Raw: value}
	return nil
}
