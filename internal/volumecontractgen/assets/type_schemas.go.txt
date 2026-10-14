package volumetype

import "github.com/selectel/blockstorage-go/pkg/v1/internal/pagination"

type View struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`

	IsPublic bool `json:"is_public"`

	// ExtraSpecs contains only specs visible to the token.
	ExtraSpecs map[string]string `json:"extra_specs"`
}

type QoSLimitsView struct {
	VolumeTypeID    string         `json:"volume_type_id"`
	QoSSpecs        map[string]int `json:"qos_specs"`
	AllowUserQoS    bool           `json:"allow_user_qos"`
	FullQoSDiskType bool           `json:"full_qos_disk_type"`
}

type viewEnvelope struct {
	VolumeType View `json:"volume_type"`
}

type pageEnvelope struct {
	VolumeTypes []View            `json:"volume_types"`
	Links       []pagination.Link `json:"volume_type_links"`
}

func (p *pageEnvelope) Items() []View { return p.VolumeTypes }

func (p *pageEnvelope) NextHref() string { return pagination.NextHref(p.Links) }
