package images

import (
	"encoding/json"
	"regexp"
	"time"

	"github.com/festy23/selectel-sdk/types"
)

var offsetTimestamp = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]+)?(?:Z|[+-](?:[01][0-9]|2[0-3]):[0-5][0-9])$`)

// Timestamp preserves timezone-less provider text without assigning an offset.
// Offset-bearing values expose an Instant and serialize as RFC3339Nano in UTC.
type Timestamp struct {
	raw     string
	instant *time.Time
}

func (t Timestamp) String() string {
	if t.instant != nil {
		return t.instant.Format(time.RFC3339Nano)
	}
	return t.raw
}

// Instant returns a UTC instant only when the provider supplied a timezone.
func (t Timestamp) Instant() (time.Time, bool) {
	if t.instant == nil {
		return time.Time{}, false
	}
	return *t.instant, true
}
func (t Timestamp) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }
func (t *Timestamp) UnmarshalJSON(raw []byte) error {
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return types.NewError(types.ProtocolError)
	}
	if parsed, e := time.Parse(time.RFC3339Nano, text); e == nil && offsetTimestamp.MatchString(text) {
		instant := parsed.UTC()
		*t = Timestamp{raw: text, instant: &instant}
		return nil
	}
	// Parsing validates the calendar/time syntax; its assumed location is never
	// retained or exposed as an instant when the provider omitted the offset.
	if _, e := time.Parse("2006-01-02T15:04:05.999999999", text); e == nil {
		*t = Timestamp{raw: text}
		return nil
	}
	return types.NewError(types.ProtocolError)
}
