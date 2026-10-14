package subnet

import (
	"context"
	"net/http"
	"net/url"

	"github.com/selectel/vpc-go/internal/api"

	vpc "github.com/selectel/vpc-go/pkg/v2"
)

const collectionPath = "/v2.0/subnets"

type AllocationPool struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

type HostRoute struct {
	Destination string `json:"destination"`
	NextHop     string `json:"nexthop"`
}

type Subnet struct {
	ID              string           `json:"id"`
	NetworkID       string           `json:"network_id"`
	IPVersion       int              `json:"ip_version"`
	CIDR            string           `json:"cidr"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	GatewayIP       *string          `json:"gateway_ip"`
	AllocationPools []AllocationPool `json:"allocation_pools"`
	DNSNameservers  []string         `json:"dns_nameservers"`
	HostRoutes      []HostRoute      `json:"host_routes"`
	EnableDHCP      bool             `json:"enable_dhcp"`
	IPv6RAMode      *string          `json:"ipv6_ra_mode"`
	IPv6AddressMode *string          `json:"ipv6_address_mode"`
	ProjectID       string           `json:"project_id"`
	ServiceTypes    []string         `json:"service_types"`
	RevisionNumber  int              `json:"revision_number"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
	Tags            []string         `json:"tags"`
	Blocked         bool             `json:"blocked"`
}

type CreateRequest struct {
	NetworkID       string                `json:"network_id"`
	IPVersion       int                   `json:"ip_version"`
	CIDR            *string               `json:"cidr,omitempty"`
	Name            *string               `json:"name,omitempty"`
	Description     *string               `json:"description,omitempty"`
	GatewayIP       *vpc.Optional[string] `json:"gateway_ip,omitempty"`
	AllocationPools *[]AllocationPool     `json:"allocation_pools,omitempty"`
	DNSNameservers  *[]string             `json:"dns_nameservers,omitempty"`
	HostRoutes      *[]HostRoute          `json:"host_routes,omitempty"`
	EnableDHCP      *bool                 `json:"enable_dhcp,omitempty"`
	IPv6RAMode      *string               `json:"ipv6_ra_mode,omitempty"`
	IPv6AddressMode *string               `json:"ipv6_address_mode,omitempty"`
	ProjectID       *string               `json:"project_id,omitempty"`
}

type UpdateRequest struct {
	Name            *string               `json:"name,omitempty"`
	Description     *string               `json:"description,omitempty"`
	GatewayIP       *vpc.Optional[string] `json:"gateway_ip,omitempty"`
	AllocationPools *[]AllocationPool     `json:"allocation_pools,omitempty"`
	DNSNameservers  *[]string             `json:"dns_nameservers,omitempty"`
	HostRoutes      *[]HostRoute          `json:"host_routes,omitempty"`
	EnableDHCP      *bool                 `json:"enable_dhcp,omitempty"`
}

type envelope struct {
	Subnet Subnet `json:"subnet"`
}

type listEnvelope struct {
	Subnets []Subnet       `json:"subnets"`
	Links   []api.PageLink `json:"subnets_links"`
}

func Create(ctx context.Context, client *vpc.Client, request CreateRequest) (*Subnet, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodPost, collectionPath, nil,
		struct {
			Subnet CreateRequest `json:"subnet"`
		}{Subnet: request},
		&result,
		http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}
	return &result.Subnet, nil
}

func Get(ctx context.Context, client *vpc.Client, subnetID string) (*Subnet, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodGet, resourcePath(subnetID), nil, nil, &result,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &result.Subnet, nil
}

func Update(
	ctx context.Context,
	client *vpc.Client,
	subnetID string,
	request UpdateRequest,
) (*Subnet, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodPut, resourcePath(subnetID), nil,
		struct {
			Subnet UpdateRequest `json:"subnet"`
		}{Subnet: request},
		&result,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &result.Subnet, nil
}

func Delete(ctx context.Context, client *vpc.Client, subnetID string) error {
	return api.Request(ctx, client, http.MethodDelete, resourcePath(subnetID), nil, nil, nil,
		http.StatusNoContent,
	)
}

func List(
	ctx context.Context,
	client *vpc.Client,
	options vpc.ListOptions,
) ([]Subnet, error) {
	return api.WalkPages(ctx, options.Values(), func(
		ctx context.Context,
		query url.Values,
	) (api.Page[Subnet], error) {
		var result listEnvelope
		err := api.Request(ctx, client, http.MethodGet, collectionPath, query, nil, &result,
			http.StatusOK,
		)
		return api.Page[Subnet]{
			Items:    result.Subnets,
			NextLink: api.NextPageLink(result.Links),
		}, err
	})
}

type Tags struct {
	operations api.TagOperations
}

func TagOperations(client *vpc.Client, subnetID string) Tags {
	return Tags{operations: api.NewTagOperations(
		client, "subnets", subnetID,
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

func resourcePath(subnetID string) string {
	return collectionPath + "/" + url.PathEscape(subnetID)
}
