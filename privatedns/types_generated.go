// Code generated from Private DNS OpenAPI SHA256 0ea46bb63f01ff9cab69085bbbe55284bb10ad1f5fa97c4e9b65d61d85f68d46; DO NOT EDIT.
package privatedns

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
)

type Service struct {
	ID                *string            `json:"id,omitempty"`
	Project           *string            `json:"project,omitempty"`
	NetworkID         *string            `json:"network_id,omitempty"`
	HighAvailability  *bool              `json:"high_availability,omitempty"`
	IsRecursorEnabled *bool              `json:"is_recursor_enabled,omitempty"`
	Ref               *types.ResourceRef `json:"ref,omitempty"`
}
type ServiceDetails struct {
	ID                *string            `json:"id,omitempty"`
	Project           *string            `json:"project,omitempty"`
	NetworkID         *string            `json:"network_id,omitempty"`
	HighAvailability  *bool              `json:"high_availability,omitempty"`
	IsRecursorEnabled *bool              `json:"is_recursor_enabled,omitempty"`
	Addresses         []Address          `json:"addresses,omitempty"`
	Ref               *types.ResourceRef `json:"ref,omitempty"`
}
type Address struct {
	Address *string `json:"address,omitempty"`
	CIDR    *string `json:"cidr,omitempty"`
}
type ServiceCreate struct {
	NetworkID         string `json:"network_id"`
	IsRecursorEnabled *bool  `json:"is_recursor_enabled,omitempty"`
}

func (v ServiceCreate) wire() map[string]any {
	m := map[string]any{}
	m["network_id"] = v.NetworkID
	if v.IsRecursorEnabled != nil {
		m["is_recursor_enabled"] = *v.IsRecursorEnabled
	}
	return m
}

type ServiceUpdate struct {
	IsRecursorEnabled *bool `json:"is_recursor_enabled,omitempty"`
}

func (v ServiceUpdate) wire() map[string]any {
	m := map[string]any{}
	if v.IsRecursorEnabled != nil {
		m["is_recursor_enabled"] = *v.IsRecursorEnabled
	}
	return m
}

type Zone struct {
	ID           *string            `json:"id,omitempty"`
	Name         *string            `json:"name,omitempty"`
	Domain       *string            `json:"domain,omitempty"`
	Project      *string            `json:"project,omitempty"`
	ReservedBy   *string            `json:"reserved_by,omitempty"`
	TTL          *types.Integer     `json:"ttl,omitempty"`
	SerialNumber *types.Integer     `json:"serial_number,omitempty"`
	Ref          *types.ResourceRef `json:"ref,omitempty"`
}
type ZoneDetails struct {
	ID           *string            `json:"id,omitempty"`
	Name         *string            `json:"name,omitempty"`
	Domain       *string            `json:"domain,omitempty"`
	Project      *string            `json:"project,omitempty"`
	ReservedBy   *string            `json:"reserved_by,omitempty"`
	TTL          *types.Integer     `json:"ttl,omitempty"`
	SerialNumber *types.Integer     `json:"serial_number,omitempty"`
	Records      []RecordSet        `json:"records,omitempty"`
	Bindings     []ZoneBinding      `json:"bindings,omitempty"`
	Ref          *types.ResourceRef `json:"ref,omitempty"`
}
type ZoneBinding struct {
	ResourceID   *string `json:"resource_id,omitempty"`
	ResourceType *string `json:"resource_type,omitempty"`
}
type ZoneCreate struct {
	Name    string          `json:"name"`
	Domain  string          `json:"domain"`
	TTL     *types.Integer  `json:"ttl,omitempty"`
	Records *[]RecordSetAdd `json:"records,omitempty"`
}

func (v ZoneCreate) wire() map[string]any {
	m := map[string]any{}
	m["name"] = v.Name
	m["domain"] = v.Domain
	if v.TTL != nil {
		m["ttl"] = *v.TTL
	}
	if v.Records != nil {
		m["records"] = wireSlice(*v.Records)
	}
	return m
}

type ZoneUpdate struct {
	Name *string        `json:"name,omitempty"`
	TTL  *types.Integer `json:"ttl,omitempty"`
}

func (v ZoneUpdate) wire() map[string]any {
	m := map[string]any{}
	if v.Name != nil {
		m["name"] = *v.Name
	}
	if v.TTL != nil {
		m["ttl"] = *v.TTL
	}
	return m
}

type RecordSet struct {
	Type      *string                `json:"type,omitempty"`
	Domain    *string                `json:"domain,omitempty"`
	Generated *bool                  `json:"generated,omitempty"`
	TTL       *types.Integer         `json:"ttl,omitempty"`
	Values    []types.ProtectedValue `json:"values,omitempty"`
}

func (v *RecordSet) UnmarshalJSON(raw []byte) error {
	type plain RecordSet
	var x struct {
		Type      *string        `json:"type"`
		Domain    *string        `json:"domain"`
		TTL       *types.Integer `json:"ttl"`
		Generated *bool          `json:"generated"`
		Values    []string       `json:"values"`
	}
	if json.Unmarshal(raw, &x) != nil {
		return types.NewError(types.ProtocolError)
	}
	v.Type = x.Type
	v.Domain = x.Domain
	v.TTL = x.TTL
	v.Generated = x.Generated
	v.Values = nil
	for _, s := range x.Values {
		v.Values = append(v.Values, types.Protect([]byte(s)))
	}
	return nil
}

type RecordSetAdd struct {
	Type   string                  `json:"type"`
	Domain string                  `json:"domain"`
	TTL    *types.Integer          `json:"ttl,omitempty"`
	Values *[]types.ProtectedValue `json:"values,omitempty"`
}

func (v RecordSetAdd) wire() map[string]any {
	m := map[string]any{}
	m["type"] = v.Type
	m["domain"] = v.Domain
	if v.TTL != nil {
		m["ttl"] = *v.TTL
	}
	if v.Values != nil {
		m["values"] = wireValues(*v.Values)
	}
	return m
}

type RecordSetDelete struct {
	Type   string `json:"type"`
	Domain string `json:"domain"`
}

func (v RecordSetDelete) wire() map[string]any {
	m := map[string]any{}
	m["type"] = v.Type
	m["domain"] = v.Domain
	return m
}

type RecordSetReplace struct {
	Generated *bool              `json:"generated,omitempty"`
	Set       *[]RecordSetAdd    `json:"set,omitempty"`
	Delete    *[]RecordSetDelete `json:"delete,omitempty"`
}

func (v RecordSetReplace) wire() map[string]any {
	m := map[string]any{}
	if v.Generated != nil {
		m["generated"] = *v.Generated
	}
	if v.Set != nil {
		m["set"] = wireSlice(*v.Set)
	}
	if v.Delete != nil {
		m["delete"] = wireSlice(*v.Delete)
	}
	return m
}

type RecordSetPatch struct {
	Generated *bool              `json:"generated,omitempty"`
	Records   *[]RecordPatchItem `json:"records,omitempty"`
}

func (v RecordSetPatch) wire() map[string]any {
	m := map[string]any{}
	if v.Generated != nil {
		m["generated"] = *v.Generated
	}
	if v.Records != nil {
		m["records"] = wireSlice(*v.Records)
	}
	return m
}

type RecordPatchItem struct {
	Type   string                  `json:"type"`
	Domain string                  `json:"domain"`
	TTL    *types.Integer          `json:"ttl,omitempty"`
	Add    *[]types.ProtectedValue `json:"add,omitempty"`
	Delete *[]types.ProtectedValue `json:"delete,omitempty"`
}

func (v RecordPatchItem) wire() map[string]any {
	m := map[string]any{}
	m["type"] = v.Type
	m["domain"] = v.Domain
	if v.TTL != nil {
		m["ttl"] = *v.TTL
	}
	if v.Add != nil {
		m["add"] = wireValues(*v.Add)
	}
	if v.Delete != nil {
		m["delete"] = wireValues(*v.Delete)
	}
	return m
}
func wireSlice[T interface{ wire() map[string]any }](v []T) []map[string]any {
	o := make([]map[string]any, len(v))
	for i := range v {
		o[i] = v[i].wire()
	}
	return o
}
func wireValues(v []types.ProtectedValue) []string {
	o := make([]string, len(v))
	for i := range v {
		o[i] = string(v[i].Reveal())
	}
	return o
}

var recordTypes = map[string]bool{"A": true, "AAAA": true, "MX": true, "CNAME": true, "TXT": true}
