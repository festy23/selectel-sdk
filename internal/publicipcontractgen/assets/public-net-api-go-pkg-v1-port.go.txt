package v1

import (
	"context"
	"net/http"
	"net/url"
	"path"
	"time"
)

type portResponse struct {
	Port *Port `json:"port"`
}

type portsListResponse struct {
	Ports []*Port `json:"ports"`
}

type Port struct {
	ID               string    `json:"id"`
	ProjectID        string    `json:"project_id"`
	NetworkID        string    `json:"network_id"`
	IPAddress        string    `json:"ip_address"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Description      *string   `json:"description"`
	Subnet           string    `json:"subnet"`
	Gateway          string    `json:"gateway"`
	SecurityGroupIDs []string  `json:"security_group_ids"`
	AdminStateUp     bool      `json:"admin_state_up"`
}

type PortCreateDTO struct {
	ProjectID        string   `json:"project_id"`
	Description      *string  `json:"description,omitempty"`
	SecurityGroupIDs []string `json:"security_group_ids"`
	AdminStateUp     *bool    `json:"admin_state_up,omitempty"`
}

type PortUpdateDTO struct {
	Description      *string  `json:"description,omitempty"`
	SecurityGroupIDs []string `json:"security_group_ids"`
	AdminStateUp     *bool    `json:"admin_state_up,omitempty"`
}

type PortsQuery struct {
	ProjectID   string
	NetworkID   string
	Description string
}

func (q *PortsQuery) toValues() url.Values {
	if q == nil {
		return nil
	}
	vals := url.Values{}

	if q.ProjectID != "" {
		vals.Set("project_id", q.ProjectID)
	}
	if q.NetworkID != "" {
		vals.Set("network_id", q.NetworkID)
	}
	if q.Description != "" {
		vals.Set("description", q.Description)
	}

	return vals
}

type PortDeleteQuery struct {
	Force bool
}

func (q *PortDeleteQuery) toValues() url.Values {
	if q == nil {
		return nil
	}
	vals := url.Values{}

	if q.Force {
		vals.Set("force", "true")
	}

	return vals
}

func (client *PublicNetAPIClient) ListPorts(
	ctx context.Context,
	query *PortsQuery,
) ([]*Port, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/public_ports", nil, query.toValues())
	if err != nil {
		return nil, err
	}

	ports := &portsListResponse{}

	err = client.doRequest(req, http.StatusOK, ports)
	if err != nil {
		return nil, err
	}

	return ports.Ports, nil
}

func (client *PublicNetAPIClient) GetPort(
	ctx context.Context,
	portID string,
) (*Port, error) {
	reqPath := path.Join("/public_ports", portID)
	req, err := client.makeRequest(ctx, http.MethodGet, reqPath, nil, nil)
	if err != nil {
		return nil, err
	}

	port := &portResponse{}

	err = client.doRequest(req, http.StatusOK, port)
	if err != nil {
		return nil, err
	}

	return port.Port, nil
}

func (client *PublicNetAPIClient) CreatePort(
	ctx context.Context,
	dto *PortCreateDTO,
) (*Port, error) {
	req, err := client.makeRequest(ctx, http.MethodPost, "/public_ports", dto, nil)
	if err != nil {
		return nil, err
	}

	port := &portResponse{}

	err = client.doRequest(req, http.StatusCreated, port)
	if err != nil {
		return nil, err
	}

	return port.Port, nil
}

func (client *PublicNetAPIClient) UpdatePort(
	ctx context.Context,
	portID string,
	dto *PortUpdateDTO,
) (*Port, error) {
	reqPath := path.Join("/public_ports", portID)
	req, err := client.makeRequest(ctx, http.MethodPatch, reqPath, dto, nil)
	if err != nil {
		return nil, err
	}

	port := &portResponse{}

	err = client.doRequest(req, http.StatusOK, port)
	if err != nil {
		return nil, err
	}

	return port.Port, nil
}

func (client *PublicNetAPIClient) DeletePort(
	ctx context.Context,
	portID string,
	query *PortDeleteQuery,
) error {
	reqPath := path.Join("/public_ports", portID)
	req, err := client.makeRequest(ctx, http.MethodDelete, reqPath, nil, query.toValues())
	if err != nil {
		return err
	}

	return client.doRequest(req, http.StatusNoContent, nil)
}
