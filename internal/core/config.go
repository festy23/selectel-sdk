package core

import (
	"net"
	"net/http"
	"reflect"
	"time"

	"github.com/festy23/selectel-sdk/credentials"
	"github.com/festy23/selectel-sdk/types"
)

// Observer sees allowlisted metadata only. Callbacks must not block.
type Observer func(Event)
type Event struct {
	OperationID          string
	Scope                types.Scope
	APIGeneration        string
	EndpointID           string
	Attempt              uint
	Latency              time.Duration
	HTTPStatus           int
	Code                 types.Code
	RequestIDs           []string
	Outcome              types.Outcome
	PageCount, ItemCount uint
	Complete             bool
}

type Config struct {
	AccountID                    string
	Credentials                  credentials.Source
	Transport                    http.RoundTripper
	EndpointOverrides            map[types.EndpointKey]types.EndpointBinding
	CallTimeout                  time.Duration
	TransferTimeout              time.Duration
	CollectionTimeout            time.Duration
	ReadMaxAttempts              uint
	RetryBaseDelay               time.Duration
	RetryMaxDelay                time.Duration
	RefreshSkew                  *time.Duration
	AuthCacheEntries             uint
	PageSize, MaxItems, MaxPages uint
	MaxJSONBytes                 uint64
	WaitTimeout, PollInterval    time.Duration
	MaxMetricSamples             uint
	Observer                     Observer
}

func normalize(c Config) (Config, error) {
	if c.AccountID == "" || nilValue(c.Credentials) {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	defaults := []struct {
		p *time.Duration
		d time.Duration
	}{
		{&c.CallTimeout, 30 * time.Second}, {&c.TransferTimeout, 30 * time.Minute}, {&c.CollectionTimeout, 10 * time.Minute},
		{&c.RetryBaseDelay, 250 * time.Millisecond}, {&c.RetryMaxDelay, 2 * time.Second}, {&c.WaitTimeout, 10 * time.Minute}, {&c.PollInterval, 2 * time.Second},
	}
	for _, s := range defaults {
		if *s.p < 0 {
			return Config{}, types.NewError(types.InvalidArgument)
		}
		if *s.p == 0 {
			*s.p = s.d
		}
	}
	if c.ReadMaxAttempts == 0 {
		c.ReadMaxAttempts = 3
	}
	if c.ReadMaxAttempts > 5 {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	if c.AuthCacheEntries == 0 {
		c.AuthCacheEntries = 128
	}
	if c.AuthCacheEntries > 1024 {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	if c.PageSize == 0 {
		c.PageSize = 100
	}
	if c.PageSize > 1000 {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	if c.MaxItems == 0 {
		c.MaxItems = 1000
	}
	if c.MaxPages == 0 {
		c.MaxPages = 100
	}
	if c.MaxJSONBytes == 0 {
		c.MaxJSONBytes = 16 * 1024 * 1024
	}
	if c.MaxMetricSamples == 0 {
		c.MaxMetricSamples = 10000
	}
	if c.RetryMaxDelay < c.RetryBaseDelay {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	skew := 60 * time.Second
	if c.RefreshSkew != nil {
		skew = *c.RefreshSkew
	}
	if skew < 0 {
		return Config{}, types.NewError(types.InvalidArgument)
	}
	c.RefreshSkew = &skew
	if nilValue(c.Transport) {
		c.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, DialContext: (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext, ForceAttemptHTTP2: true, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, TLSHandshakeTimeout: 10 * time.Second, ExpectContinueTimeout: time.Second}
	} else if t, ok := c.Transport.(*http.Transport); ok {
		copy := t.Clone()
		if copy.TLSClientConfig != nil && copy.TLSClientConfig.InsecureSkipVerify {
			return Config{}, types.NewError(types.InvalidArgument)
		}
		c.Transport = copy
	}
	copyOverrides := make(map[types.EndpointKey]types.EndpointBinding, len(c.EndpointOverrides))
	for k, v := range c.EndpointOverrides {
		if k.Service == "" || k.APIGeneration == "" || v.ID == "" || v.APIGeneration != k.APIGeneration || v.Scope != k.Scope || v.Audience == "" {
			return Config{}, types.NewError(types.ScopeMismatch)
		}
		if err := k.Scope.Validate(); err != nil {
			return Config{}, err
		}
		if err := types.ValidateEndpoint(v.URL); err != nil {
			return Config{}, err
		}
		copyOverrides[k] = v
	}
	c.EndpointOverrides = copyOverrides
	return c, nil
}

func nilValue(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return r.IsNil()
	}
	return false
}
