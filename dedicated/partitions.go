package dedicated

import (
	"encoding/json"
	"reflect"

	"github.com/festy23/selectel-sdk/types"
)

// PartitionConfiguration keeps published literal-wildcard and native SDK map
// variants explicit. Both are typed from their pinned sources; no raw payload.
type PartitionConfiguration struct {
	Published *PartitionsConfigModel               `json:"published,omitempty"`
	Native    map[string]PartitionsConfigNodeModel `json:"native,omitempty"`
}

func NativePartitions(value map[string]PartitionsConfigNodeModel) *PartitionConfiguration {
	return &PartitionConfiguration{Native: value}
}
func PublishedPartitions(value PartitionsConfigModel) *PartitionConfiguration {
	return &PartitionConfiguration{Published: &value}
}
func (v PartitionConfiguration) Validate() error {
	if (v.Published == nil) == (v.Native == nil) {
		return types.NewError(types.ProtocolError)
	}
	if v.Published != nil {
		if e := v.Published.Validate(); e != nil {
			return e
		}
		for _, node := range v.Published.Wildcard {
			if e := node.Validate(); e != nil {
				return e
			}
		}
	}
	for _, node := range v.Native {
		if e := node.Validate(); e != nil {
			return e
		}
	}
	return nil
}
func (v *PartitionConfiguration) normalize() {}
func (v *PartitionConfiguration) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return types.NewError(types.ProtocolError)
	}
	if _, ok := fields["*"]; ok {
		var value PartitionsConfigModel
		if json.Unmarshal(raw, &value) != nil {
			return types.NewError(types.ProtocolError)
		}
		*v = PartitionConfiguration{Published: &value}
	} else {
		var value map[string]PartitionsConfigNodeModel
		if json.Unmarshal(raw, &value) != nil {
			return types.NewError(types.ProtocolError)
		}
		*v = PartitionConfiguration{Native: value}
	}
	return v.Validate()
}
func (v PartitionConfiguration) MarshalJSON() ([]byte, error) {
	if e := v.Validate(); e != nil {
		return nil, e
	}
	if v.Published != nil {
		return json.Marshal(v.Published)
	}
	return json.Marshal(v.Native)
}
func (v PartitionConfiguration) wireValue() (any, error) {
	if e := v.Validate(); e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if v.Published != nil {
		return wireValue(reflect.ValueOf(v.Published))
	}
	return wireValue(reflect.ValueOf(v.Native))
}
