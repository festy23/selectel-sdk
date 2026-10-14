package dedicated

import (
	"encoding/json"
	"net/url"

	"github.com/festy23/selectel-sdk/types"
)

func queryValues(value interface{ Validate() error }) (url.Values, error) {
	if inputError(value) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	raw, e := requestBody(value)
	if e != nil {
		return nil, e
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	q := url.Values{}
	for k, raw := range fields {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			q.Set(k, text)
		} else {
			q.Set(k, string(raw))
		}
	}
	return q, nil
}
