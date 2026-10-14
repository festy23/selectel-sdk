# blockstorage-go: Go SDK for the Selectel Block Storage API

Package `blockstorage-go` provides a Go SDK to work with the Selectel public Block Storage API,
which is an OpenStack Cinder v3 API.

The `pkg/v1` import path identifies the first version of the SDK's public Go contract. It does
not identify the remote API version; operations in this package use Cinder API v3.

## Status

This README targets the `v0.2.0` release. Releases in the `v0.x` series may introduce
breaking public API changes before `v1.0.0`.

## Documentation

The public Go API documentation is available at
[pkg.go.dev](https://pkg.go.dev/github.com/selectel/blockstorage-go/pkg/v1).

## Supported operations

The `volume` package provides typed operations to:

* create, get, update, extend and delete a volume;
* list all volumes with their details.

The read-only `volumetype` package provides typed operations to:

* get a volume type by UUID or get the configured default type;
* list all volume types and their user-visible extra specs;
* list the available QoS capabilities and match them to volume types by `VolumeTypeID`.

The read-only `snapshot` package provides typed operations to:

* get a snapshot by UUID;
* list all snapshots, optionally filtered by name, status and volume UUID.

The SDK accepts a regional Cinder v3 endpoint, a project-scoped token, a `User-Agent` and an
HTTP client from the caller. It does not perform Keystone authentication, poll for resource state
transitions or retry requests on its own. The default HTTP client has a 120-second request timeout;
the caller can replace it through `Config.HTTPClient` and use contexts for shorter deadlines.

`Create`, `Get`, `Update`, `Extend`, `Delete` and `ListQoSLimits` perform one HTTP request.
`volume.ListDetail`, `volumetype.List` and `snapshot.List` follow pagination and may perform one
request per page. All listings return `v1.KindIncompleteList` if any page cannot be read, so a
partial result is never mistaken for the complete list.

The SDK sends a Cinder microversion only when an operation requires one: `Create` from a backup
uses `3.47`, and extending an attached volume uses `3.42`. Generic request dispatch, arbitrary
query parameters and arbitrary microversion controls remain internal, so the public API cannot
retype a volume or change its attachments.

The default client and every concrete `*http.Client` supplied through `Config.HTTPClient` refuse
redirects. Pagination links are reanchored to the configured endpoint. A custom implementation of
the `HTTPClient` interface owns its redirect policy.

Waiting for state transitions, retries and lifecycle decisions belongs to the caller, for example
to `terraform-provider-selectel`.

## Getting started

### Installation

```bash
go get github.com/selectel/blockstorage-go/pkg/v1/volume@v0.2.0
```

Install `github.com/selectel/blockstorage-go/pkg/v1/volumetype@v0.2.0` or
`github.com/selectel/blockstorage-go/pkg/v1/snapshot@v0.2.0` as well when the application works
with volume types, QoS capabilities or snapshots.

### Authentication

The SDK does not authenticate. Obtain a project-scoped Keystone token and a regional
`volumev3` endpoint from the service catalog first, for example with
[go-selvpcclient](https://github.com/selectel/go-selvpcclient), and pass them to the client.

### Endpoints

Use the regional `volumev3` endpoint returned by the Keystone service catalog. The available
regional API endpoints are listed in the
[Selectel documentation](https://docs.selectel.ru/api/urls/).

The client is built once and passed to the operations of a resource package.

### Usage example

```go
package main

import (
	"context"
	"log"
	"os"

	v1 "github.com/selectel/blockstorage-go/pkg/v1"
	"github.com/selectel/blockstorage-go/pkg/v1/volume"
)

func main() {
	endpoint := os.Getenv("OS_VOLUME_ENDPOINT")
	token := os.Getenv("OS_TOKEN")
	if endpoint == "" || token == "" {
		log.Fatal("OS_VOLUME_ENDPOINT and OS_TOKEN must be set")
	}

	client, err := v1.NewClient(v1.Config{
		Endpoint:  endpoint,
		Token:     token,
		UserAgent: "my-application/v1.0.0",
	})
	if err != nil {
		log.Fatal(err)
	}

	created, response, err := volume.Create(context.Background(), client, volume.CreateOpts{
		Size:             10,
		Name:             "data",
		VolumeType:       "universal.ru-1a",
		AvailabilityZone: "ru-1a",
	})
	if err != nil {
		log.Fatal(err)
	}

	// The volume is still creating: waiting for a status belongs to the caller.
	log.Println(created.ID, created.Status, response.RequestID)
}
```

The configured default volume type is read without knowing its name, and the answer carries the
actual identifier of the type it resolves to:

```go
found, _, err := volumetype.Get(ctx, client, volumetype.DefaultTypeID)
```

Note that a deployment may accept volume type names at volume creation that `volumetype.List`
does not return, so an absent name does not prove that a volume cannot be created with it.

## Errors

Every failure is reported as a `*v1.Error` with a `Kind` that classifies it, and with
the available status code, request ID and a safe diagnostic message. Use `v1.IsKind`
to branch on the error class instead of parsing messages:

```go
if v1.IsKind(err, v1.KindNotFound) {
	// The resource is missing for the current scope.
}
```

The classes are `KindNotFound`, `KindForbidden`, `KindConflict`, `KindInvalidRequest`,
`KindOverQuota`, `KindRateLimited`, `KindServerError`, `KindTransport`, `KindTimeout`,
`KindCanceled`, `KindMicroversion`, `KindIncompleteList` and `KindUnexpected`.

`v1.KindOverQuota` and `KindRateLimited` are deliberately separate. An exhausted quota is not
recoverable by repeating the request: the quota has to be raised or the occupied resources have
to be released, which requires a decision of the operator or of the administrator. A throttled
request can succeed later, so the caller may recover from it on its own.

The API answers `413` both for an exhausted quota and for oversized input such as too large
metadata, and only the quota fault carries a `retryAfter` field. The SDK uses that field to
report the first case as `KindOverQuota` and the second as `KindInvalidRequest`. The API does
not throttle at all, so `KindRateLimited` only appears when a proxy in front of it answers
`429`.

Note that the API answers `404` for a volume that exists but is not visible to the token, with
a body that is identical to the answer for a volume that never existed. An absence therefore
has to be confirmed by the caller before it is acted upon.

## Development

```text
pkg/v1              the public client facade, configuration and error classes
pkg/v1/internal     the transport and pagination implementation
pkg/v1/snapshot     the read-only operations over snapshots
pkg/v1/volume       the operations over a volume and over the listing of volumes
pkg/v1/volumetype   the read-only operations over volume types
```

```bash
make        # lint + unit tests
make lint   # golangci-lint run ./...
make test   # go test ./...
make cover  # coverage report
```

`make lint` requires `golangci-lint` v2.11 or later built with Go 1.25, the same version that CI
uses. An older binary fails with `the Go language version used to build golangci-lint is lower
than the targeted Go version`. Without a suitable binary the linter can be run directly:

```bash
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.11.0 run ./...
```
