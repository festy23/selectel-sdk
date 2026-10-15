package v1

import (
	"context"
	"net/http"
	"net/url"
)

type zoneDetailsContainer struct {
	Zone *ZoneDetails `json:"zone"`
}

type zonesListContainer struct {
	Zones []*Zone `json:"zones"`
}

type Zone struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Domain       string `json:"domain"`
	Project      string `json:"project"`
	ReservedBy   string `json:"reserved_by"`
	TTL          int    `json:"ttl"`
	SerialNumber int    `json:"serial_number"`
}

type ZoneBindings struct {
	ResourceID   string `json:"resource_id"`
	ResourceType string `json:"resource_type"`
}

type ZoneDetails struct {
	Zone
	Records  []*Record       `json:"records"`
	Bindings []*ZoneBindings `json:"bindings"`
}

type ZoneUpdateDto struct {
	TTL *int `json:"ttl,omitempty"`
}

type ZoneCreateDTO struct {
	Name    string          `json:"name"`
	Domain  string          `json:"domain"`
	TTL     *int            `json:"ttl,omitempty"`
	Records []*RecordSetDTO `json:"records,omitempty"`
}

type ZonesQuery struct {
	Domain  string
	Project string
}

func (q *ZonesQuery) toValues() url.Values {
	if q == nil {
		return nil
	}
	vals := url.Values{}

	if q.Domain != "" {
		vals.Set("domain", q.Domain)
	}

	return vals
}

func (client *PrivateDNSClient) ListZones(ctx context.Context, query *ZonesQuery) ([]*Zone, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/zones", nil, query.toValues())
	if err != nil {
		return nil, err
	}
	zones := &zonesListContainer{}

	return zones.Zones, client.doRequest(req, http.StatusOK, zones)
}

func (client *PrivateDNSClient) GetZone(ctx context.Context, zoneID string) (*ZoneDetails, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/zones/"+zoneID, nil, nil)
	if err != nil {
		return nil, err
	}

	zone := &zoneDetailsContainer{}

	return zone.Zone, client.doRequest(req, http.StatusOK, zone)
}

func (client *PrivateDNSClient) CreateZone(ctx context.Context, zoneDTO *ZoneCreateDTO) (*ZoneDetails, error) {
	req, err := client.makeRequest(ctx, http.MethodPost, "/zones", zoneDTO, nil)
	if err != nil {
		return nil, err
	}

	zone := &zoneDetailsContainer{}

	return zone.Zone, client.doRequest(req, http.StatusCreated, zone)
}

func (client *PrivateDNSClient) DeleteZone(ctx context.Context, zoneID string) error {
	req, err := client.makeRequest(ctx, http.MethodDelete, "/zones/"+zoneID, nil, nil)
	if err != nil {
		return err
	}

	return client.doRequest(req, http.StatusNoContent, nil)
}

func (client *PrivateDNSClient) UpdateZone(ctx context.Context, zoneID string, zoneDTO *ZoneUpdateDto) error {
	req, err := client.makeRequest(ctx, http.MethodPut, "/zones/"+zoneID, zoneDTO, nil)
	if err != nil {
		return err
	}

	return client.doRequest(req, http.StatusOK, nil)
}
