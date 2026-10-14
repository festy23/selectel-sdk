package floatingip

import (
	"context"
	"net/http"
	"net/url"

	"github.com/selectel/vpc-go/internal/api"

	vpc "github.com/selectel/vpc-go/pkg/v2"
)

const collectionPath = "/v2.0/floatingips"

type FloatingIP struct {
	ID                string  `json:"id"`
	FloatingIPAddress string  `json:"floating_ip_address"`
	PortID            *string `json:"port_id"`
	FixedIPAddress    *string `json:"fixed_ip_address"`
	Status            string  `json:"status"`
}

type UpdateRequest struct {
	PortID         *vpc.Optional[string] `json:"port_id,omitempty"`
	FixedIPAddress *vpc.Optional[string] `json:"fixed_ip_address,omitempty"`
}

type envelope struct {
	FloatingIP FloatingIP `json:"floatingip"`
}
type listEnvelope struct {
	FloatingIPs []FloatingIP   `json:"floatingips"`
	Links       []api.PageLink `json:"floatingips_links"`
}

func Get(ctx context.Context, client *vpc.Client, id string) (*FloatingIP, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodGet, resourcePath(id), nil, nil, &result,
		http.StatusOK)
	if err != nil {
		return nil, err
	}
	return &result.FloatingIP, nil
}

func Update(ctx context.Context, client *vpc.Client, id string, request UpdateRequest) (*FloatingIP, error) {
	var result envelope
	err := api.Request(ctx, client, http.MethodPut, resourcePath(id), nil,
		struct {
			FloatingIP UpdateRequest `json:"floatingip"`
		}{FloatingIP: request}, &result,
		http.StatusOK)
	if err != nil {
		return nil, err
	}
	return &result.FloatingIP, nil
}

func List(ctx context.Context, client *vpc.Client, options vpc.ListOptions) ([]FloatingIP, error) {
	return api.WalkPages(ctx, options.Values(), func(ctx context.Context, q url.Values) (api.Page[FloatingIP], error) {
		var result listEnvelope
		err := api.Request(ctx, client, http.MethodGet, collectionPath, q, nil, &result, http.StatusOK)
		return api.Page[FloatingIP]{Items: result.FloatingIPs, NextLink: api.NextPageLink(result.Links)}, err
	})
}

func resourcePath(id string) string { return collectionPath + "/" + url.PathEscape(id) }
