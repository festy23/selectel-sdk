// Code generated from pinned Selectel blockstorage-go f229309640ab15be64dcb12703d26d3a706f5abc; DO NOT EDIT.
package volumes

import "github.com/festy23/selectel-sdk/types"

type Volume struct {
	ID     string `json:"id"`
	Status string `json:"status"`

	// Size is the volume size in GiB.
	Size             types.Integer `json:"size"`
	Name             string        `json:"name"`
	Description      string        `json:"description"`
	AvailabilityZone string        `json:"availability_zone"`

	// VolumeType can differ from the requested name when Cinder resolves a regional type.
	VolumeType  string `json:"volume_type"`
	SnapshotID  string `json:"snapshot_id"`
	SourceVolID string `json:"source_volid"`

	// Metadata may include service keys added by Cinder.
	Metadata map[string]string `json:"metadata"`

	// Attachments are read-only in this SDK.
	Attachments []Attachment `json:"attachments"`

	// Bootable is returned by Cinder as the string "true" or "false".
	Bootable string             `json:"bootable"`
	Ref      *types.ResourceRef `json:"ref,omitempty"`
}
type Attachment struct {
	// ID is the volume ID. AttachmentID identifies the attachment.
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	ServerID     string `json:"server_id"`
	Device       string `json:"device"`
}
type VolumeType struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	IsPublic bool `json:"is_public"`

	// ExtraSpecs contains only specs visible to the token.
	ExtraSpecs map[string]string  `json:"extra_specs"`
	Ref        *types.ResourceRef `json:"ref,omitempty"`
}
type QoSLimits struct {
	VolumeTypeID    string                   `json:"volume_type_id"`
	QoSSpecs        map[string]types.Integer `json:"qos_specs"`
	AllowUserQoS    bool                     `json:"allow_user_qos"`
	FullQoSDiskType bool                     `json:"full_qos_disk_type"`
}
type CreateSpec struct {
	// Size is the requested size in GiB. Nil is omitted; supplied values must be positive.
	Size             *types.Integer `json:"size,omitempty"`
	Name             string         `json:"name,omitempty"`
	Description      string         `json:"description,omitempty"`
	AvailabilityZone string         `json:"availability_zone,omitempty"`

	// Metadata keys and values are limited to 255 characters by Cinder.
	Metadata    map[string]string `json:"metadata,omitempty"`
	VolumeType  string            `json:"volume_type,omitempty"`
	SnapshotID  string            `json:"snapshot_id,omitempty"`
	SourceVolID string            `json:"source_volid,omitempty"`
	ImageID     string            `json:"imageRef,omitempty"`

	// BackupID creates the volume from a backup using Cinder microversion 3.47.
	BackupID string `json:"backup_id,omitempty"`
}
type UpdateSpec struct {
	Name        *string            `json:"name,omitempty"`
	Description *string            `json:"description,omitempty"`
	Metadata    *map[string]string `json:"metadata,omitempty"`
}
