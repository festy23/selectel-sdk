package port

import (
	"context"
	"net/http"
	"net/url"

	"github.com/selectel/vpc-go/internal/api"

	vpc "github.com/selectel/vpc-go/pkg/v2"
)

const collectionPath = "/v2.0/ports"

type FixedIP struct {
	SubnetID  string `json:"subnet_id,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
}

type AllowedAddressPair struct {
	IPAddress  string `json:"ip_address"`
	MACAddress string `json:"mac_address,omitempty"`
}

type ExtraDHCPOption struct {
	Name      string `json:"opt_name"`
	Value     string `json:"opt_value"`
	IPVersion int    `json:"ip_version,omitempty"`
}

type UpdateExtraDHCPOption struct {
	Name      string  `json:"opt_name"`
	Value     *string `json:"opt_value"`
	IPVersion *int    `json:"ip_version,omitempty"`
}

type Port struct {
	ID                  string               `json:"id"`
	NetworkID           string               `json:"network_id"`
	Name                string               `json:"name"`
	Description         string               `json:"description"`
	Status              string               `json:"status"`
	AdminStateUp        bool                 `json:"admin_state_up"`
	MACAddress          string               `json:"mac_address"`
	FixedIPs            []FixedIP            `json:"fixed_ips"`
	DeviceID            string               `json:"device_id"`
	DeviceOwner         string               `json:"device_owner"`
	SecurityGroups      []string             `json:"security_groups"`
	AllowedAddressPairs []AllowedAddressPair `json:"allowed_address_pairs"`
	ExtraDHCPOptions    []ExtraDHCPOption    `json:"extra_dhcp_opts"`
	PortSecurityEnabled bool                 `json:"port_security_enabled"`
	ProjectID           string               `json:"project_id"`
	RevisionNumber      int                  `json:"revision_number"`
	CreatedAt           string               `json:"created_at"`
	UpdatedAt           string               `json:"updated_at"`
	Tags                []string             `json:"tags"`
	Blocked             bool                 `json:"blocked"`
	DHCPBlocked         bool                 `json:"dhcp_blocked"`
	DNSName             string               `json:"dns_name,omitempty"`
}

type CreateRequest struct {
	NetworkID           string                `json:"network_id"`
	Name                *string               `json:"name,omitempty"`
	Description         *string               `json:"description,omitempty"`
	AdminStateUp        *bool                 `json:"admin_state_up,omitempty"`
	MACAddress          *string               `json:"mac_address,omitempty"`
	FixedIPs            *[]FixedIP            `json:"fixed_ips,omitempty"`
	DeviceID            *string               `json:"device_id,omitempty"`
	DeviceOwner         *string               `json:"device_owner,omitempty"`
	SecurityGroups      *[]string             `json:"security_groups,omitempty"`
	AllowedAddressPairs *[]AllowedAddressPair `json:"allowed_address_pairs,omitempty"`
	ExtraDHCPOptions    *[]ExtraDHCPOption    `json:"extra_dhcp_opts,omitempty"`
	ProjectID           *string               `json:"project_id,omitempty"`
	DNSName             *string               `json:"dns_name,omitempty"`
}

type UpdateRequest struct {
	Name                *string                  `json:"name,omitempty"`
	Description         *string                  `json:"description,omitempty"`
	AdminStateUp        *bool                    `json:"admin_state_up,omitempty"`
	FixedIPs            *[]FixedIP               `json:"fixed_ips,omitempty"`
	DeviceID            *string                  `json:"device_id,omitempty"`
	DeviceOwner         *string                  `json:"device_owner,omitempty"`
	SecurityGroups      *[]string                `json:"security_groups,omitempty"`
	AllowedAddressPairs *[]AllowedAddressPair    `json:"allowed_address_pairs,omitempty"`
	ExtraDHCPOptions    *[]UpdateExtraDHCPOption `json:"extra_dhcp_opts,omitempty"`
	DNSName             *string                  `json:"dns_name,omitempty"`
}

type envelope struct {
	Port Port `json:"port"`
}

type listEnvelope struct {
	Ports []Port         `json:"ports"`
	Links []api.PageLink `json:"ports_links"`
}

func Create(ctx context.Context, client *vpc.Client, request CreateRequest) (*Port, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodPost, collectionPath, nil,
		struct {
			Port CreateRequest `json:"port"`
		}{Port: request},
		&result, http.StatusCreated,
	)
	if err != nil {
		return nil, err
	}
	return &result.Port, nil
}

func Get(ctx context.Context, client *vpc.Client, id string) (*Port, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodGet, resourcePath(id), nil, nil, &result,
		http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &result.Port, nil
}

func Update(ctx context.Context, client *vpc.Client, id string, request UpdateRequest) (*Port, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodPut, resourcePath(id), nil,
		struct {
			Port UpdateRequest `json:"port"`
		}{Port: request},
		&result, http.StatusOK,
	)
	if err != nil {
		return nil, err
	}
	return &result.Port, nil
}

func Delete(ctx context.Context, client *vpc.Client, id string) error {
	return api.Request(ctx, client, http.MethodDelete, resourcePath(id), nil, nil, nil,
		http.StatusNoContent,
	)
}

func List(ctx context.Context, client *vpc.Client, options vpc.ListOptions) ([]Port, error) {
	return api.WalkPages(ctx, options.Values(), func(
		ctx context.Context, query url.Values,
	) (api.Page[Port], error) {
		var result listEnvelope
		err := api.Request(ctx, client, http.MethodGet, collectionPath, query, nil, &result,
			http.StatusOK,
		)
		return api.Page[Port]{Items: result.Ports, NextLink: api.NextPageLink(result.Links)}, err
	})
}

type Tags struct{ operations api.TagOperations }

func TagOperations(client *vpc.Client, id string) Tags {
	return Tags{api.NewTagOperations(client, "ports", id)}
}
func (tags Tags) Get(ctx context.Context) ([]string, error) { return tags.operations.Get(ctx) }
func (tags Tags) Has(ctx context.Context, tag string) (bool, error) {
	return tags.operations.Has(ctx, tag)
}
func (tags Tags) Add(ctx context.Context, tag string) error { return tags.operations.Add(ctx, tag) }
func (tags Tags) Delete(ctx context.Context, tag string) error {
	return tags.operations.Delete(ctx, tag)
}

func (tags Tags) Replace(ctx context.Context, values []string) ([]string, error) {
	return tags.operations.Replace(ctx, values)
}
func (tags Tags) DeleteAll(ctx context.Context) error { return tags.operations.DeleteAll(ctx) }

func resourcePath(id string) string { return collectionPath + "/" + url.PathEscape(id) }
