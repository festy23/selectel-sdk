package volume

import "github.com/selectel/blockstorage-go/pkg/v1/internal/pagination"

type View struct {
	ID     string `json:"id"`
	Status string `json:"status"`

	// Size is the volume size in GiB.
	Size             int    `json:"size"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	AvailabilityZone string `json:"availability_zone"`

	// VolumeType can differ from the requested name when Cinder resolves a regional type.
	VolumeType  string `json:"volume_type"`
	SnapshotID  string `json:"snapshot_id"`
	SourceVolID string `json:"source_volid"`

	// Metadata may include service keys added by Cinder.
	Metadata map[string]string `json:"metadata"`

	// Attachments are read-only in this SDK.
	Attachments []Attachment `json:"attachments"`

	// Bootable is returned by Cinder as the string "true" or "false".
	Bootable string `json:"bootable"`
}

type Attachment struct {
	// ID is the volume ID. AttachmentID identifies the attachment.
	ID           string `json:"id"`
	AttachmentID string `json:"attachment_id"`
	ServerID     string `json:"server_id"`
	Device       string `json:"device"`
}

type viewEnvelope struct {
	Volume View `json:"volume"`
}

type pageEnvelope struct {
	Volumes []View            `json:"volumes"`
	Links   []pagination.Link `json:"volumes_links"`
}

func (p *pageEnvelope) Items() []View { return p.Volumes }

func (p *pageEnvelope) NextHref() string { return pagination.NextHref(p.Links) }
