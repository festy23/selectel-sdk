package network

import (
	"context"
	"net/http"
	"net/url"

	"github.com/selectel/vpc-go/internal/api"

	vpc "github.com/selectel/vpc-go/pkg/v2"
)

const collectionPath = "/v2.0/networks"

type Network struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Description           string   `json:"description"`
	Status                string   `json:"status"`
	AdminStateUp          bool     `json:"admin_state_up"`
	Subnets               []string `json:"subnets"`
	ProjectID             string   `json:"project_id"`
	Shared                bool     `json:"shared"`
	RouterExternal        bool     `json:"router:external"`
	MTU                   int      `json:"mtu"`
	PortSecurityEnabled   bool     `json:"port_security_enabled"`
	AvailabilityZones     []string `json:"availability_zones"`
	AvailabilityZoneHints []string `json:"availability_zone_hints"`
	RevisionNumber        int      `json:"revision_number"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
	Tags                  []string `json:"tags"`
	Blocked               bool     `json:"blocked"`
	IsPublic              bool     `json:"is_public"`
	IsDNSEnabled          bool     `json:"is_dns_enabled"`
	DNSDomain             string   `json:"dns_domain"`
}

type CreateRequest struct {
	Name                  *string   `json:"name,omitempty"`
	Description           *string   `json:"description,omitempty"`
	AdminStateUp          *bool     `json:"admin_state_up,omitempty"`
	AvailabilityZoneHints *[]string `json:"availability_zone_hints,omitempty"`
	ProjectID             *string   `json:"project_id,omitempty"`
	DNSDomain             *string   `json:"dns_domain,omitempty"`
}

type UpdateRequest struct {
	Name         *string `json:"name,omitempty"`
	Description  *string `json:"description,omitempty"`
	AdminStateUp *bool   `json:"admin_state_up,omitempty"`
	DNSDomain    *string `json:"dns_domain,omitempty"`
}

type networkEnvelope struct {
	Network Network `json:"network"`
}

type listEnvelope struct {
	Networks []Network      `json:"networks"`
	Links    []api.PageLink `json:"networks_links"`
}

func Create(ctx context.Context, client *vpc.Client, request CreateRequest) (*Network, error) {
	var envelope networkEnvelope
	err := api.Request(ctx, client,
		http.MethodPost,
		collectionPath,
		nil,
		struct {
			Network CreateRequest `json:"network"`
		}{Network: request},
		&envelope,
		http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}
	return &envelope.Network, nil
}

func Get(ctx context.Context, client *vpc.Client, networkID string) (*Network, error) {
	var envelope networkEnvelope
	err := api.Request(ctx, client,
		http.MethodGet,
		resourcePath(networkID),
		nil,
		nil,
		&envelope,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &envelope.Network, nil
}

func Update(
	ctx context.Context,
	client *vpc.Client,
	networkID string,
	request UpdateRequest,
) (*Network, error) {
	var envelope networkEnvelope
	err := api.Request(ctx, client,
		http.MethodPut,
		resourcePath(networkID),
		nil,
		struct {
			Network UpdateRequest `json:"network"`
		}{Network: request},
		&envelope,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &envelope.Network, nil
}

func Delete(ctx context.Context, client *vpc.Client, networkID string) error {
	return api.Request(ctx, client,
		http.MethodDelete,
		resourcePath(networkID),
		nil,
		nil,
		nil,
		http.StatusNoContent,
	)
}

func List(
	ctx context.Context,
	client *vpc.Client,
	options vpc.ListOptions,
) ([]Network, error) {
	return api.WalkPages(ctx, options.Values(), func(
		ctx context.Context,
		query url.Values,
	) (api.Page[Network], error) {
		var envelope listEnvelope
		err := api.Request(ctx, client,
			http.MethodGet,
			collectionPath,
			query,
			nil,
			&envelope,
			http.StatusOK,
		)
		return api.Page[Network]{
			Items:    envelope.Networks,
			NextLink: api.NextPageLink(envelope.Links),
		}, err
	})
}

type Tags struct {
	operations api.TagOperations
}

func TagOperations(client *vpc.Client, networkID string) Tags {
	return Tags{operations: api.NewTagOperations(
		client,
		"networks",
		networkID,
	)}
}

func (tags Tags) Get(ctx context.Context) ([]string, error) {
	return tags.operations.Get(ctx)
}

func (tags Tags) Has(ctx context.Context, tag string) (bool, error) {
	return tags.operations.Has(ctx, tag)
}

func (tags Tags) Add(ctx context.Context, tag string) error {
	return tags.operations.Add(ctx, tag)
}

func (tags Tags) Delete(ctx context.Context, tag string) error {
	return tags.operations.Delete(ctx, tag)
}

func (tags Tags) Replace(ctx context.Context, values []string) ([]string, error) {
	return tags.operations.Replace(ctx, values)
}

func (tags Tags) DeleteAll(ctx context.Context) error {
	return tags.operations.DeleteAll(ctx)
}

func resourcePath(networkID string) string {
	return collectionPath + "/" + url.PathEscape(networkID)
}
