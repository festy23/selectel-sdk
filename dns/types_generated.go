// Code generated from current DNS OpenAPI SHA256 0658efafdfba61ca7c0da4ec5ad3a20d848c4695d3a44f598c809ea56b769177; DO NOT EDIT.
package dns

import "github.com/festy23/selectel-sdk/types"
import "encoding/json"

type ZoneCreate struct {
	Name string `json:"name"`
}
type Zone struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	ProjectID           string             `json:"project_id"`
	CreatedAt           *string            `json:"created_at,omitempty"`
	UpdatedAt           *string            `json:"updated_at,omitempty"`
	Comment             *string            `json:"comment,omitempty"`
	Disabled            *bool              `json:"disabled,omitempty"`
	Protected           *bool              `json:"protected,omitempty"`
	DelegationCheckedAt *string            `json:"delegation_checked_at,omitempty"`
	LastDelegatedAt     *string            `json:"last_delegated_at,omitempty"`
	LastCheckStatus     *bool              `json:"last_check_status,omitempty"`
	Ref                 *types.ResourceRef `json:"ref,omitempty"`
}
type RRSetCreate struct {
	Name    string                  `json:"name"`
	TTL     types.Integer           `json:"ttl"`
	Type    AllowedRecordTypes      `json:"type"`
	Records []Records               `json:"records"`
	Comment *types.Nullable[string] `json:"comment,omitempty"`
}
type RRSet struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	TTL       types.Integer      `json:"ttl"`
	Type      AllowedRecordTypes `json:"type"`
	Records   []Records          `json:"records"`
	Comment   *string            `json:"comment,omitempty"`
	ManagedBy *string            `json:"managed_by,omitempty"`
	ZoneID    string             `json:"zone_id"`
	Ref       *types.ResourceRef `json:"ref,omitempty"`
}
type RRSetUpdate struct {
	TTL     types.Integer           `json:"ttl"`
	Records []Records               `json:"records"`
	Comment *types.Nullable[string] `json:"comment,omitempty"`
}
type ZonesSortingOptions string

const (
	ZonesSortingOptionsNameAscend       ZonesSortingOptions = "name.ascend"
	ZonesSortingOptionsNameDescend      ZonesSortingOptions = "name.descend"
	ZonesSortingOptionsCreatedAtAscend  ZonesSortingOptions = "created_at.ascend"
	ZonesSortingOptionsCreatedAtDescend ZonesSortingOptions = "created_at.descend"
	ZonesSortingOptionsUpdatedAtAscend  ZonesSortingOptions = "updated_at.ascend"
	ZonesSortingOptionsUpdatedAtDescend ZonesSortingOptions = "updated_at.descend"
)

type RRSetSortingOptions string

const (
	RRSetSortingOptionsTypeAscend  RRSetSortingOptions = "type.ascend"
	RRSetSortingOptionsTypeDescend RRSetSortingOptions = "type.descend"
	RRSetSortingOptionsNameAscend  RRSetSortingOptions = "name.ascend"
	RRSetSortingOptionsNameDescend RRSetSortingOptions = "name.descend"
)

type AllowedRecordTypes string

const (
	AllowedRecordTypesA     AllowedRecordTypes = "A"
	AllowedRecordTypesAAAA  AllowedRecordTypes = "AAAA"
	AllowedRecordTypesALIAS AllowedRecordTypes = "ALIAS"
	AllowedRecordTypesCAA   AllowedRecordTypes = "CAA"
	AllowedRecordTypesCNAME AllowedRecordTypes = "CNAME"
	AllowedRecordTypesDNAME AllowedRecordTypes = "DNAME"
	AllowedRecordTypesHTTPS AllowedRecordTypes = "HTTPS"
	AllowedRecordTypesMX    AllowedRecordTypes = "MX"
	AllowedRecordTypesNS    AllowedRecordTypes = "NS"
	AllowedRecordTypesSOA   AllowedRecordTypes = "SOA"
	AllowedRecordTypesSRV   AllowedRecordTypes = "SRV"
	AllowedRecordTypesSVCB  AllowedRecordTypes = "SVCB"
	AllowedRecordTypesSSHFP AllowedRecordTypes = "SSHFP"
	AllowedRecordTypesTXT   AllowedRecordTypes = "TXT"
)

type Records struct {
	Content  types.ProtectedValue `json:"content"`
	Disabled *bool                `json:"disabled,omitempty"`
}

func (v *Records) UnmarshalJSON(raw []byte) error {
	var w struct {
		Content  *string `json:"content"`
		Disabled *bool   `json:"disabled"`
	}
	if json.Unmarshal(raw, &w) != nil || w.Content == nil {
		return types.NewError(types.ProtocolError)
	}
	v.Content = types.Protect([]byte(*w.Content))
	v.Disabled = w.Disabled
	return nil
}
