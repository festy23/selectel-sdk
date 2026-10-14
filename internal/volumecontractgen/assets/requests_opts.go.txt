package volume

type CreateOpts struct {
	// Size is the requested size in GiB. Zero is omitted from the request.
	Size             int
	Name             string
	Description      string
	AvailabilityZone string

	// Metadata keys and values are limited to 255 characters by Cinder.
	Metadata    map[string]string
	VolumeType  string
	SnapshotID  string
	SourceVolID string
	ImageID     string

	// BackupID creates the volume from a backup using Cinder microversion 3.47.
	BackupID string
}

// Nil fields are not changed. Metadata replaces the existing map instead of merging it.
type UpdateOpts struct {
	Name        *string
	Description *string
	Metadata    map[string]string
}

func (opts UpdateOpts) isEmpty() bool {
	return opts.Name == nil && opts.Description == nil && opts.Metadata == nil
}

type ExtendOpts struct {
	// NewSize is the new size in GiB. Cinder does not support shrinking volumes.
	NewSize int

	// Attached enables online resize using Cinder microversion 3.42.
	Attached bool
}

type ListOpts struct {
	// Limit is the page size. ListDetail still reads all pages.
	Limit int
}

type createRequest struct {
	Volume createBody `json:"volume"`
}

type createBody struct {
	Size             int               `json:"size,omitempty"`
	Name             string            `json:"name,omitempty"`
	Description      string            `json:"description,omitempty"`
	AvailabilityZone string            `json:"availability_zone,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	VolumeType       string            `json:"volume_type,omitempty"`
	SnapshotID       string            `json:"snapshot_id,omitempty"`
	SourceVolID      string            `json:"source_volid,omitempty"`
	ImageID          string            `json:"imageRef,omitempty"`
	BackupID         string            `json:"backup_id,omitempty"`
}

type updateRequest struct {
	Volume updateBody `json:"volume"`
}

type updateBody struct {
	Name        *string            `json:"name,omitempty"`
	Description *string            `json:"description,omitempty"`
	Metadata    *map[string]string `json:"metadata,omitempty"`
}

type extendRequest struct {
	Extend extendBody `json:"os-extend"`
}

type extendBody struct {
	NewSize int `json:"new_size"`
}
