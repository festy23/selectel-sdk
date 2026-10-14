package compute

import (
	"encoding/json"
	"github.com/festy23/selectel-sdk/types"
)

func deviceMap(value any, connection *types.ProtectedValue, noDevice *types.ProtectedJSON) (map[string]json.RawMessage, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, types.NewError(types.InvalidArgument)
	}
	if connection != nil {
		fields["connection_info"], _ = json.Marshal(string(connection.Reveal()))
	}
	if noDevice != nil {
		raw := noDevice.Reveal()
		if !json.Valid(raw) {
			return nil, types.NewError(types.InvalidArgument)
		}
		fields["no_device"] = json.RawMessage(raw)
	}
	return fields, nil
}
func applyDeviceVariants(server map[string]any, r CreateRequest, minor int) error {
	if r.BlockDevices != nil && (r.LegacyBlockDevices != nil || len(r.Options.BlockDevice) > 0) || r.LegacyBlockDevices != nil && len(r.Options.BlockDevice) > 0 {
		return types.NewError(types.InvalidArgument)
	}
	if r.NetworkAttachments != nil {
		networks := make([]NetworkAttachment, len(r.NetworkAttachments))
		copy(networks, r.NetworkAttachments)
		for _, n := range networks {
			if minor >= 37 && n.UUID != nil && !novaUUID.MatchString(*n.UUID) {
				return types.NewError(types.InvalidArgument)
			}
			port := false
			if n.Port != nil {
				v, valid := n.Port.Value()
				port = valid && v != ""
			}
			network := n.UUID != nil && *n.UUID != ""
			if network == port {
				return types.NewError(types.InvalidArgument)
			}
			if n.Tag != nil && !(minor >= 32 && minor <= 36 || minor >= 42) {
				return types.NewError(types.InvalidArgument)
			}
		}
		server["networks"] = networks
	}
	if r.BlockDevices != nil {
		devices := make([]map[string]json.RawMessage, 0, len(r.BlockDevices))
		for _, b := range r.BlockDevices {
			if b.Tag != nil && !(minor == 32 || minor >= 42) || b.VolumeType != nil && minor < 67 {
				return types.NewError(types.InvalidArgument)
			}
			if b.SourceType != nil && *b.SourceType != "image" && *b.SourceType != "volume" && *b.SourceType != "snapshot" && *b.SourceType != "blank" {
				return types.NewError(types.InvalidArgument)
			}
			if b.DestinationType != nil && *b.DestinationType != "local" && *b.DestinationType != "volume" {
				return types.NewError(types.InvalidArgument)
			}
			fields, e := deviceMap(b, b.ConnectionInfo, b.NoDevice)
			if e != nil {
				return e
			}
			devices = append(devices, fields)
		}
		server["block_device_mapping_v2"] = devices
	}
	if r.LegacyBlockDevices != nil {
		devices := make([]map[string]json.RawMessage, 0, len(r.LegacyBlockDevices))
		for _, b := range r.LegacyBlockDevices {
			fields, e := deviceMap(b, b.ConnectionInfo, b.NoDevice)
			if e != nil {
				return e
			}
			devices = append(devices, fields)
		}
		server["block_device_mapping"] = devices
	}
	return nil
}
