package v1

import (
	"context"
	"net/http"
)

type servicesListContainer struct {
	Services []*Service `json:"services"`
}

type servicesDetailsContainer struct {
	Service *ServiceDetails `json:"service"`
}

type Service struct {
	ID                string `json:"id"`
	Project           string `json:"project"`
	NetworkID         string `json:"network_id"`
	HighAvailability  bool   `json:"high_availability"`
	IsRecursorEnabled bool   `json:"is_recursor_enabled"`
}

type ServiceDetails struct {
	Service
	Addresses []*ServiceAddress `json:"addresses"`
}

type ServiceCreateDTO struct {
	NetworkID         string `json:"network_id"`
	IsRecursorEnabled *bool  `json:"is_recursor_enabled"`
}

type ServiceUpdateDTO struct {
	ServiceID         string `json:"-"`
	IsRecursorEnabled *bool  `json:"is_recursor_enabled"`
}

type ServiceAddress struct {
	Address string `json:"address"`
	CIDR    string `json:"cidr"`
}

func (client *PrivateDNSClient) ListServices(ctx context.Context) ([]*Service, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/services", nil, nil)
	if err != nil {
		return nil, err
	}

	services := &servicesListContainer{}

	return services.Services, client.doRequest(req, http.StatusOK, services)
}

func (client *PrivateDNSClient) GetService(ctx context.Context, serviceID string) (*ServiceDetails, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/services/"+serviceID, nil, nil)
	if err != nil {
		return nil, err
	}

	service := &servicesDetailsContainer{}

	return service.Service, client.doRequest(req, http.StatusOK, service)
}

func (client *PrivateDNSClient) UpdateService(ctx context.Context, dto *ServiceUpdateDTO) error {
	req, err := client.makeRequest(ctx, http.MethodPut, "/services/"+dto.ServiceID, dto, nil)
	if err != nil {
		return err
	}

	return client.doRequest(req, http.StatusNoContent, nil)
}

func (client *PrivateDNSClient) CreateService(ctx context.Context, serviceDTO *ServiceCreateDTO) (*ServiceDetails, error) {
	req, err := client.makeRequest(ctx, http.MethodPost, "/services", serviceDTO, nil)
	if err != nil {
		return nil, err
	}

	service := &servicesDetailsContainer{}

	return service.Service, client.doRequest(req, http.StatusCreated, service)
}

func (client *PrivateDNSClient) DeleteService(ctx context.Context, serviceID string) error {
	req, err := client.makeRequest(ctx, http.MethodDelete, "/services/"+serviceID, nil, nil)
	if err != nil {
		return err
	}

	return client.doRequest(req, http.StatusNoContent, nil)
}
