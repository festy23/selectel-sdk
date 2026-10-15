package registry

import (
	"bytes"
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
	"math/big"
)

func registryRows[T any](raw []byte, attach func(*T) error) ([]T, error) {
	var rows []json.RawMessage
	if json.Unmarshal(raw, &rows) != nil || rows == nil {
		return nil, types.NewError(types.ProtocolError)
	}
	out := []T{}
	for _, row := range rows {
		var value T
		if bytes.Equal(bytes.TrimSpace(row), []byte("null")) || json.Unmarshal(row, &value) != nil {
			return out, types.NewError(types.ProtocolError)
		}
		if validator, ok := any(value).(interface{ Validate() error }); ok {
			if validator.Validate() != nil {
				return out, types.NewError(types.ProtocolError)
			}
		}
		if attach != nil {
			if attach(&value) != nil {
				return out, types.NewError(types.ProtocolError)
			}
		}
		out = append(out, value)
	}
	return out, nil
}
func registryEnvelope(raw []byte, key string) (json.RawMessage, *types.Integer, *types.Integer, error) {
	var v map[string]json.RawMessage
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return nil, nil, nil, types.NewError(types.ProtocolError)
	}
	var total, filtered *types.Integer
	if json.Unmarshal(v["totalCount"], &total) != nil || json.Unmarshal(v["filteredCount"], &filtered) != nil || total == nil || filtered == nil {
		return nil, nil, nil, types.NewError(types.ProtocolError)
	}
	all, ok := new(big.Int).SetString(total.String(), 10)
	n, valid := new(big.Int).SetString(filtered.String(), 10)
	if !ok || !valid || all.Sign() < 0 || n.Sign() < 0 || n.Cmp(all) > 0 {
		return nil, nil, nil, types.NewError(types.ProtocolError)
	}
	return v[key], total, filtered, nil
}
