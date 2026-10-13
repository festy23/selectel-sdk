package core

import (
	"context"
	"github.com/festy23/selectel-sdk/catalog"
	"github.com/festy23/selectel-sdk/types"
	"net/http"
	"sync"
)

type Runtime struct {
	config  Config
	http    *http.Client
	catalog catalog.Catalog
	mu      sync.Mutex
	cache   map[authKey]*authEntry
	flights map[authKey]*authFlight
	latest  map[credentialKey]string
	clock   uint64
}

func New(config Config) (*Runtime, error) {
	cfg, err := normalize(config)
	if err != nil {
		return nil, err
	}
	r := &Runtime{config: cfg, catalog: catalog.Default(), cache: map[authKey]*authEntry{}, flights: map[authKey]*authFlight{}, latest: map[credentialKey]string{}}
	r.http = &http.Client{Transport: cfg.Transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return r, nil
}

func (r *Runtime) Catalog() catalog.Catalog { return r.catalog }
func (r *Runtime) Authenticate(ctx context.Context, request types.AuthRequest) (types.AuthMetadata, error) {
	if ctx == nil {
		return types.AuthMetadata{}, types.NewError(types.InvalidArgument)
	}
	call, cancel := context.WithTimeout(ctx, r.config.CallTimeout)
	defer cancel()
	entry, err := r.authorize(call, request)
	if err != nil {
		return types.AuthMetadata{}, err
	}
	metadata := entry.metadata
	metadata.Scope = request.Scope
	if metadata.ExpiresAt != nil {
		copy := *metadata.ExpiresAt
		metadata.ExpiresAt = &copy
	}
	return metadata, nil
}
