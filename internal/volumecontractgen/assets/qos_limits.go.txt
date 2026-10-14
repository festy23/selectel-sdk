package volumetype

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	v1 "github.com/selectel/blockstorage-go/pkg/v1"
	"github.com/selectel/blockstorage-go/pkg/v1/internal/transport"
)

const qosLimitsPath = "qos-specs/qos_limits"

// ListQoSLimits returns QoS settings for volume types. The response also contains service fields,
// which are ignored. Use VolumeTypeID to match settings with a volume type.
func ListQoSLimits(ctx context.Context, client *v1.Client) ([]QoSLimitsView, *v1.Response, error) {
	raw := make(map[string]json.RawMessage)
	response, err := transport.DoRequest(
		ctx,
		client,
		http.MethodGet,
		qosLimitsPath,
		http.StatusOK,
		nil,
		&raw,
	)
	if err != nil {
		return nil, response, err
	}
	if len(raw) == 0 {
		return nil, response, unexpectedQoSLimitsResponse(response, "the response body has no members", nil)
	}

	type namedView struct {
		name string
		view QoSLimitsView
	}

	views := make([]namedView, 0, len(raw))
	for name, value := range raw {
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			continue
		}

		var view QoSLimitsView
		if err := json.Unmarshal(trimmed, &view); err != nil {
			return nil, response, unexpectedQoSLimitsResponse(
				response,
				fmt.Sprintf("unable to decode QoS limits for volume type entry %q", name),
				err,
			)
		}
		if view.VolumeTypeID == "" {
			return nil, response, unexpectedQoSLimitsResponse(
				response,
				fmt.Sprintf("QoS limits entry %q has no volume_type_id", name),
				nil,
			)
		}

		views = append(views, namedView{name: name, view: view})
	}

	sort.Slice(views, func(i, j int) bool {
		if views[i].view.VolumeTypeID == views[j].view.VolumeTypeID {
			return views[i].name < views[j].name
		}

		return views[i].view.VolumeTypeID < views[j].view.VolumeTypeID
	})

	result := make([]QoSLimitsView, len(views))
	for i := range views {
		result[i] = views[i].view
	}

	return result, response, nil
}

func unexpectedQoSLimitsResponse(response *v1.Response, message string, err error) *v1.Error {
	result := &v1.Error{Kind: v1.KindUnexpected, Message: message, Err: err}
	if response != nil {
		result.StatusCode = response.StatusCode
		result.RequestID = response.RequestID
	}

	return result
}
