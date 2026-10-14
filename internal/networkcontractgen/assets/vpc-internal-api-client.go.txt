package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/selectel/vpc-go/internal/utils"
)

const (
	authTokenHeader = "X-Auth-Token" // #nosec G101 -- This is an HTTP header name.
	userAgentHeader = "User-Agent"
)

var moduleUserAgent = "vpc-go/" + utils.ModuleVersion

type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

type Config struct {
	Endpoint   string
	Token      string
	UserAgent  string
	HTTPClient HTTPClient
}

type Client struct {
	endpoint   *url.URL
	token      string
	userAgent  string
	httpClient HTTPClient
}

func NewClient(config Config) (*Client, error) {
	if config.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	if config.Token == "" {
		return nil, errors.New("token is required")
	}

	endpoint, err := url.Parse(config.Endpoint)
	if err != nil {
		return nil, err
	}
	if endpoint.Scheme == "" || endpoint.Host == "" {
		return nil, errors.New("endpoint must be an absolute URL")
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}

	userAgent := moduleUserAgent
	if config.UserAgent != "" {
		userAgent = config.UserAgent + " " + moduleUserAgent
	}

	return &Client{
		endpoint: endpoint, token: config.Token, userAgent: userAgent, httpClient: httpClient,
	}, nil
}

func (client *Client) do(
	ctx context.Context,
	method string,
	path string,
	query url.Values,
	body io.Reader,
) (*http.Response, error) {
	requestURL := client.endpoint.JoinPath(path)
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, method, requestURL.String(), body)
	if err != nil {
		return nil, newRequestError(err)
	}
	request.Header.Set(authTokenHeader, client.token)
	request.Header.Set(userAgentHeader, client.userAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, &TransportError{Err: err}
	}

	return response, nil
}
