package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/selectel/private-dns-go/pkg/utils"
)

const (
	xAuthHeader     = "x-Auth-Token"
	userAgentHeader = "User-Agent"

	defaultHTTPTimeout           = 120
	defaultDialTimeout           = 60
	defaultKeepaliveTimeout      = 60
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 100
	defaultTLSHandshakeTimeout   = 60
	defaultExpectContinueTimeout = 1
)

var moduleUserAgent = "private-dns-go/" + utils.Version

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	// URL of service API
	URL string
	// Keystone project scoped token
	AuthToken string
	// Optional, HTTPClient for process you request
	HTTPClient HTTPClient
	// Optional, additional user agent, will be added before module ser agent
	UserAgent string
}

type PrivateDNSClient struct {
	cfg *Config
}

func defaultHTTPClient() *http.Client {
	return &http.Client{
		Timeout: defaultHTTPTimeout * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   defaultDialTimeout * time.Second,
				KeepAlive: defaultKeepaliveTimeout * time.Second,
			}).DialContext,
			MaxIdleConns:          defaultMaxIdleConns,
			IdleConnTimeout:       defaultIdleConnTimeout * time.Second,
			TLSHandshakeTimeout:   defaultTLSHandshakeTimeout * time.Second,
			ExpectContinueTimeout: defaultExpectContinueTimeout * time.Second,
		},
	}
}

func NewPrivateDNSClient(cfg *Config) *PrivateDNSClient {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = defaultHTTPClient()
	}

	return &PrivateDNSClient{cfg: cfg}
}

func (client *PrivateDNSClient) makeRequest(ctx context.Context, method, path string, body any, query url.Values) (*http.Request, error) {
	reqURL, err := url.JoinPath(client.cfg.URL, path)
	if err != nil {
		return nil, NewClientErr(err)
	}
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		body, err := json.Marshal(body)
		if err != nil {
			return nil, NewClientErr(err)
		}
		payload = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, payload)
	if err != nil {
		return nil, NewClientErr(err)
	}
	req.Header.Set(xAuthHeader, client.cfg.AuthToken)

	userAgent := moduleUserAgent
	if client.cfg.UserAgent != "" {
		userAgent = client.cfg.UserAgent + " " + userAgent
	}
	req.Header.Add(userAgentHeader, userAgent)

	return req, nil
}

func (client *PrivateDNSClient) processAPIError(response *http.Response) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return NewClientErr(err)
	}

	apiErr := &APIErr{raw: data, Code: response.StatusCode}
	wrap := errWrapper{Err: apiErr}

	// If response struct invalid use raw response
	_ = json.Unmarshal(data, &wrap)

	return apiErr
}

func (client *PrivateDNSClient) unmarshalResponse(response *http.Response, target any) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return NewClientErr(err)
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return NewClientErr(err)
	}

	return nil
}

func (client *PrivateDNSClient) doRequest(req *http.Request, expectedCode int, target any) error {
	res, err := client.cfg.HTTPClient.Do(req)
	if err != nil {
		return NewTransportErr(err)
	}
	defer res.Body.Close()

	if res.StatusCode != expectedCode {
		return client.processAPIError(res)
	}

	if target != nil {
		return client.unmarshalResponse(res, target)
	}

	return nil
}
