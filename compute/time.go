package compute

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"time"
)

// NovaTime normalizes Nova's RFC3339 and legacy naive UTC timestamps.
type NovaTime struct{ time.Time }

func (t NovaTime) String() string               { return t.UTC().Format(time.RFC3339Nano) }
func (t NovaTime) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
func (t *NovaTime) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999"} {
		if parsed, e := time.Parse(layout, text); e == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return types.NewError(types.ProtocolError)
}
