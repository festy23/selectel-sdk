package volumetype

import (
	"context"
	"net/http"
	"net/url"

	v1 "github.com/selectel/blockstorage-go/pkg/v1"
	"github.com/selectel/blockstorage-go/pkg/v1/internal/pagination"
	"github.com/selectel/blockstorage-go/pkg/v1/internal/transport"
)

const (
	basePath = "types"

	// DefaultTypeID asks Cinder to resolve the configured default volume type.
	DefaultTypeID = "default"
)

type ListOpts struct {
	// Limit is the page size. List still reads all pages.
	Limit int
}

// Pass DefaultTypeID to get the configured default volume type.
func Get(ctx context.Context, client *v1.Client, volumeTypeID string) (*View, *v1.Response, error) {
	if volumeTypeID == "" {
		return nil, nil, &v1.Error{Kind: v1.KindInvalidRequest, Message: "the volume type ID is required"}
	}

	path := basePath + "/" + url.PathEscape(volumeTypeID)
	envelope := &viewEnvelope{}

	response, err := transport.DoRequest(
		ctx,
		client,
		http.MethodGet,
		path,
		http.StatusOK,
		nil,
		envelope,
		transport.WithResponseEnvelope("volume_type"),
	)
	if err != nil {
		return nil, response, err
	}

	return &envelope.VolumeType, response, nil
}

// List returns all volume types visible to the token and never returns a partial result. Cinder
// may accept type names that are not present in this list.
func List(ctx context.Context, client *v1.Client, opts ListOpts) ([]View, error) {
	return pagination.ReadAll[View](ctx, client, basePath, "volume_types", nil, opts.Limit, func() *pageEnvelope {
		return &pageEnvelope{}
	})
}
