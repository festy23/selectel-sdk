package compute

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/festy23/selectel-sdk/types"
)

var offsetTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// NovaTime preserves timezone-less provider text without assigning an offset.
// Offset-bearing values expose an Instant and serialize as RFC3339Nano in UTC.
type NovaTime struct {
	raw     string
	instant *time.Time
}

func (t NovaTime) String() string {
	if t.instant != nil {
		return t.instant.Format(time.RFC3339Nano)
	}
	return t.raw
}

// Instant returns a UTC instant only when the provider supplied a timezone.
func (t NovaTime) Instant() (time.Time, bool) {
	if t.instant == nil {
		return time.Time{}, false
	}
	return *t.instant, true
}
func (t NovaTime) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
func (t *NovaTime) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	if parsed, e := time.Parse(time.RFC3339Nano, text); e == nil && offsetTimestamp.MatchString(text) {
		instant := parsed.UTC()
		*t = NovaTime{raw: text, instant: &instant}
		return nil
	}
	// Parsing validates the calendar/time syntax; its assumed location is never
	// retained or exposed as an instant when the provider omitted the offset.
	if _, e := time.Parse("2006-01-02T15:04:05.999999999", text); e == nil {
		*t = NovaTime{raw: text}
		return nil
	}
	return types.NewError(types.ProtocolError)
}
