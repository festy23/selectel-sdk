package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/selectel/public-net-api-go/internal/utils"
)

const (
	xAuthHeader     = "X-Auth-Token"
	userAgentHeader = "User-Agent"

	defaultHTTPTimeout           = 120
	defaultDialTimeout           = 60
	defaultKeepaliveTimeout      = 60
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 100
	defaultTLSHandshakeTimeout   = 60
	defaultExpectContinueTimeout = 1
)

var moduleUserAgent = "public-net-api-go/" + utils.ModuleVersion

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	URL        string
	AuthToken  string
	HTTPClient HTTPClient
	UserAgent  string
}

type PublicNetAPIClient struct {
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

func NewPublicNetAPIClient(cfg *Config) (*PublicNetAPIClient, error) {
	if cfg == nil {
		return nil, newClientErr(errors.New("config is nil"))
	}
	if cfg.URL == "" {
		return nil, newClientErr(errors.New("url is required"))
	}
	if cfg.AuthToken == "" {
		return nil, newClientErr(errors.New("auth token is required"))
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}

	baseURL, err := url.JoinPath(cfg.URL, "v1")
	if err != nil {
		return nil, newClientErr(err)
	}

	return &PublicNetAPIClient{
		cfg: &Config{
			URL:        baseURL,
			AuthToken:  cfg.AuthToken,
			HTTPClient: httpClient,
			UserAgent:  cfg.UserAgent,
		},
	}, nil
}

func (client *PublicNetAPIClient) makeRequest(
	ctx context.Context,
	method, path string,
	body any,
	query url.Values,
) (*http.Request, error) {
	reqURL, err := url.JoinPath(client.cfg.URL, path)
	if err != nil {
		return nil, newClientErr(err)
	}
	if len(query) > 0 {
		reqURL += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		body, err := json.Marshal(body)
		if err != nil {
			return nil, newClientErr(err)
		}
		payload = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, payload)
	if err != nil {
		return nil, newClientErr(err)
	}
	req.Header.Set(xAuthHeader, client.cfg.AuthToken)

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	userAgent := moduleUserAgent
	if client.cfg.UserAgent != "" {
		userAgent = client.cfg.UserAgent + " " + userAgent
	}
	req.Header.Add(userAgentHeader, userAgent)

	return req, nil
}

func (client *PublicNetAPIClient) processAPIError(response *http.Response) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return newClientErr(err)
	}

	apiErr := &APIErr{raw: data, Code: response.StatusCode}
	wrap := errWrapper{Err: apiErr}

	_ = json.Unmarshal(data, &wrap)

	return apiErr
}

func (client *PublicNetAPIClient) unmarshalResponse(response *http.Response, target any) error {
	data, err := io.ReadAll(response.Body)
	if err != nil {
		return newClientErr(err)
	}

	err = json.Unmarshal(data, target)
	if err != nil {
		return newClientErr(err)
	}

	return nil
}

func (client *PublicNetAPIClient) doRequest(req *http.Request, expectedCode int, target any) error {
	res, err := client.cfg.HTTPClient.Do(req)
	if err != nil {
		return newTransportErr(err)
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
