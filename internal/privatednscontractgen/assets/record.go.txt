package v1

import (
	"context"
	"net/http"
	"net/url"
)

type recordsListContainer struct {
	Records []*Record `json:"records"`
}

type Record struct {
	Type      string   `json:"type"`
	Domain    string   `json:"domain"`
	TTL       int      `json:"ttl"`
	Values    []string `json:"values"`
	Generated bool     `json:"generated"`
}

type RecordDeleteDTO struct {
	Type   string `json:"type"`
	Domain string `json:"domain"`
}

type RecordSetDTO struct {
	Type   string   `json:"type"`
	Domain string   `json:"domain"`
	TTL    *int     `json:"ttl,omitempty"`
	Values []string `json:"values"`
}

type PutRecordsDTO struct {
	Set    []*RecordSetDTO    `json:"set,omitempty"`
	Delete []*RecordDeleteDTO `json:"delete,omitempty"`
}

type RecordsQuery struct {
	Type   string
	Domain string
}

func (q *RecordsQuery) toValues() url.Values {
	if q == nil {
		return nil
	}
	vals := url.Values{}

	if q.Type != "" {
		vals.Set("type", q.Type)
	}
	if q.Domain != "" {
		vals.Set("domain", q.Domain)
	}

	return vals
}

func (client *PrivateDNSClient) ListRecords(ctx context.Context, zoneID string, query *RecordsQuery) ([]*Record, error) {
	req, err := client.makeRequest(ctx, http.MethodGet, "/zones/"+zoneID+"/recordset", nil, query.toValues())
	if err != nil {
		return nil, err
	}

	records := &recordsListContainer{}

	return records.Records, client.doRequest(req, http.StatusOK, records)
}

func (client *PrivateDNSClient) PutRecords(ctx context.Context, zoneID string, recordsDTO *PutRecordsDTO) ([]*Record, error) {
	req, err := client.makeRequest(ctx, http.MethodPut, "/zones/"+zoneID+"/recordset", recordsDTO, nil)
	if err != nil {
		return nil, err
	}

	records := &recordsListContainer{}

	return records.Records, client.doRequest(req, http.StatusOK, records)
}
