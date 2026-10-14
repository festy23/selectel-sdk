package v1

import (
	"context"
	"errors"
	"net/http"
	"path"
)

type Quota struct {
	Value int `json:"value"`
	Used  int `json:"used"`
}

type ProjectQuotasResponse struct {
	NetworkDirectPublicIPs []Quota `json:"network_direct_public_ips"`
}

func (client *PublicNetAPIClient) GetQuotas(
	ctx context.Context,
	projectID string,
) (*ProjectQuotasResponse, error) {
	if projectID == "" {
		return nil, newClientErr(errors.New("project id is required"))
	}

	reqPath := path.Join("/projects", projectID, "quotas")
	req, err := client.makeRequest(ctx, http.MethodGet, reqPath, nil, nil)
	if err != nil {
		return nil, err
	}

	quotas := &ProjectQuotasResponse{}

	err = client.doRequest(req, http.StatusOK, quotas)
	if err != nil {
		return nil, err
	}

	return quotas, nil
}
