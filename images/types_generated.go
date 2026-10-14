// Code generated from pinned Selectel images-go ace640b1fd6a1bc13cf3c8d7be7a77c9777a3362; DO NOT EDIT.
package images

import "github.com/festy23/selectel-sdk/types"

type Image struct {
	ID              string                `json:"id,omitempty"`
	Name            *string               `json:"name,omitempty"`
	Status          string                `json:"status,omitempty"`
	Visibility      *string               `json:"visibility,omitempty"`
	Protected       *bool                 `json:"protected,omitempty"`
	Hidden          *bool                 `json:"os_hidden,omitempty"`
	Checksum        *string               `json:"checksum,omitempty"`
	HashAlgo        *string               `json:"os_hash_algo,omitempty"`
	HashValue       *string               `json:"os_hash_value,omitempty"`
	Owner           *string               `json:"owner,omitempty"`
	Size            *types.Integer        `json:"size,omitempty"`
	VirtualSize     *types.Integer        `json:"virtual_size,omitempty"`
	ContainerFormat *string               `json:"container_format,omitempty"`
	DiskFormat      *string               `json:"disk_format,omitempty"`
	MinDisk         *types.Integer        `json:"min_disk,omitempty"`
	MinRAM          *types.Integer        `json:"min_ram,omitempty"`
	CreatedAt       *Timestamp            `json:"created_at,omitempty"`
	UpdatedAt       *Timestamp            `json:"updated_at,omitempty"`
	Tags            []string              `json:"tags,omitempty"`
	File            *string               `json:"file,omitempty"`
	Self            *string               `json:"self,omitempty"`
	Schema          *string               `json:"schema,omitempty"`
	DirectURL       *types.ProtectedValue `json:"direct_url,omitempty"`
	Stores          *string               `json:"stores,omitempty"`
	Properties      types.ProtectedJSON   `json:"properties"`
	Locations       *types.ProtectedJSON  `json:"locations,omitempty"`
	Ref             *types.ResourceRef    `json:"ref,omitempty"`
}
type Member struct {
	MemberID  string             `json:"member_id,omitempty"`
	ImageID   string             `json:"image_id,omitempty"`
	Status    string             `json:"status,omitempty"`
	CreatedAt *Timestamp         `json:"created_at,omitempty"`
	UpdatedAt *Timestamp         `json:"updated_at,omitempty"`
	Schema    *string            `json:"schema,omitempty"`
	Ref       *types.ResourceRef `json:"ref,omitempty"`
}
type CreateSpec struct {
	Name            *types.Nullable[string]         `json:"name,omitempty"`
	ID              *string                         `json:"id,omitempty"`
	Visibility      *string                         `json:"visibility,omitempty"`
	Protected       *bool                           `json:"protected,omitempty"`
	Hidden          *bool                           `json:"os_hidden,omitempty"`
	ContainerFormat *types.Nullable[string]         `json:"container_format,omitempty"`
	DiskFormat      *types.Nullable[string]         `json:"disk_format,omitempty"`
	MinDisk         *types.Integer                  `json:"min_disk,omitempty"`
	MinRAM          *types.Integer                  `json:"min_ram,omitempty"`
	Tags            *[]string                       `json:"tags,omitempty"`
	Properties      map[string]types.ProtectedValue `json:"properties,omitempty"`
}

var baseKeys = map[string]bool{"id": true, "name": true, "status": true, "visibility": true, "protected": true, "os_hidden": true, "checksum": true, "os_hash_algo": true, "os_hash_value": true, "owner": true, "size": true, "virtual_size": true, "container_format": true, "disk_format": true, "min_disk": true, "min_ram": true, "created_at": true, "updated_at": true, "tags": true, "file": true, "self": true, "schema": true, "direct_url": true, "stores": true, "locations": true}
