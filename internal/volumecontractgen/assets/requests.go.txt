package volume

import (
	"context"
	"net/http"
	"net/url"

	v1 "github.com/selectel/blockstorage-go/pkg/v1"
	"github.com/selectel/blockstorage-go/pkg/v1/internal/pagination"
	"github.com/selectel/blockstorage-go/pkg/v1/internal/transport"
)

const (
	basePath                     = "volumes"
	detailPath                   = "volumes/detail"
	microversionOnlineResize     = "3.42"
	microversionCreateFromBackup = "3.47"
)

// Creating a volume from a backup uses Cinder microversion 3.47.
func Create(ctx context.Context, client *v1.Client, opts CreateOpts) (*View, *v1.Response, error) {
	request := createRequest{
		Volume: createBody(opts),
	}

	options := []transport.RequestOption{
		transport.WithResponseEnvelope("volume"),
	}
	if opts.BackupID != "" {
		options = append(options, transport.WithMicroversion(microversionCreateFromBackup))
	}

	envelope := &viewEnvelope{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodPost, basePath, http.StatusAccepted, request, envelope, options...,
	)
	if err != nil {
		return nil, response, err
	}

	return &envelope.Volume, response, nil
}

// Cinder returns not found for volumes hidden from the token.
func Get(ctx context.Context, client *v1.Client, volumeID string) (*View, *v1.Response, error) {
	path, err := volumePath(volumeID)
	if err != nil {
		return nil, nil, err
	}

	envelope := &viewEnvelope{}

	response, err := transport.DoRequest(
		ctx,
		client,
		http.MethodGet,
		path,
		http.StatusOK,
		nil,
		envelope,
		transport.WithResponseEnvelope("volume"),
	)
	if err != nil {
		return nil, response, err
	}

	return &envelope.Volume, response, nil
}

// Metadata replaces the existing map instead of merging it.
func Update(
	ctx context.Context, client *v1.Client, volumeID string, opts UpdateOpts,
) (*View, *v1.Response, error) {
	path, err := volumePath(volumeID)
	if err != nil {
		return nil, nil, err
	}

	if opts.isEmpty() {
		return nil, nil, &v1.Error{
			Kind:    v1.KindInvalidRequest,
			Message: "an update needs at least one of the name, the description and the metadata",
		}
	}

	requestBody := updateBody{
		Name:        opts.Name,
		Description: opts.Description,
	}
	if opts.Metadata != nil {
		requestBody.Metadata = &opts.Metadata
	}

	request := updateRequest{Volume: requestBody}

	envelope := &viewEnvelope{}

	response, err := transport.DoRequest(
		ctx,
		client,
		http.MethodPut,
		path,
		http.StatusOK,
		request,
		envelope,
		transport.WithResponseEnvelope("volume"),
	)
	if err != nil {
		return nil, response, err
	}

	return &envelope.Volume, response, nil
}

// Set ExtendOpts.Attached to use online resize.
func Extend(ctx context.Context, client *v1.Client, volumeID string, opts ExtendOpts) (*v1.Response, error) {
	path, err := volumeActionPath(volumeID)
	if err != nil {
		return nil, err
	}

	if opts.NewSize <= 0 {
		return nil, &v1.Error{
			Kind:    v1.KindInvalidRequest,
			Message: "the new size of the volume must be a positive number of gibibytes",
		}
	}

	request := extendRequest{Extend: extendBody{NewSize: opts.NewSize}}

	var options []transport.RequestOption
	if opts.Attached {
		options = append(options, transport.WithMicroversion(microversionOnlineResize))
	}

	return transport.DoRequest(ctx, client, http.MethodPost, path, http.StatusAccepted, request, nil, options...)
}

// Cinder rejects attached volumes without detaching them.
func Delete(ctx context.Context, client *v1.Client, volumeID string) (*v1.Response, error) {
	path, err := volumePath(volumeID)
	if err != nil {
		return nil, err
	}

	return transport.DoRequest(
		ctx, client, http.MethodDelete, path, http.StatusAccepted, nil, nil,
	)
}

// ListDetail returns all pages of the detailed volume list and never returns a partial result.
func ListDetail(ctx context.Context, client *v1.Client, opts ListOpts) ([]View, error) {
	return pagination.ReadAll[View](ctx, client, detailPath, "volumes", nil, opts.Limit, func() *pageEnvelope {
		return &pageEnvelope{}
	})
}

func volumePath(volumeID string) (string, error) {
	if volumeID == "" {
		return "", &v1.Error{Kind: v1.KindInvalidRequest, Message: "the volume ID is required"}
	}

	return basePath + "/" + url.PathEscape(volumeID), nil
}

func volumeActionPath(volumeID string) (string, error) {
	path, err := volumePath(volumeID)
	if err != nil {
		return "", err
	}

	return path + "/action", nil
}
