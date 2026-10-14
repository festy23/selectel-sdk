# public-net-api-go

Go SDK for the Selectel Cloud Public Net API service for managing networking resources - direct public IP addresses (which are represented as public ports).

For example, it is used the [Selectel Terraform Provider](https://github.com/selectel/terraform-provider-selectel).

## Getting started

### Installation

You can install needed `public-net-api-go` packages via `go get` command:

```bash
go get github.com/selectel/public-net-api-go
```

## Authentication

To work with the Selectel Public Net API you first need to:

* Create a Selectel account: [registration page](https://my.selectel.ru/registration).
* Create a project in Selectel Cloud Platform [projects](https://my.selectel.ru/vpc/projects).
* Retrieve a Keystone token for your project via API or [go-selvpcclient](https://github.com/selectel/go-selvpcclient).

## Endpoints

You can find available endpoints [here](https://docs.selectel.ru/en/api/urls/).

## Usage example

```go
package main

import (
	"context"
	"fmt"
	"log"

	publicnetapi "github.com/selectel/public-net-api-go/pkg/v1"
)

func main() {
	cfg := &publicnetapi.Config{
		AuthToken: "...",
		URL:       "https://ru-3.cloud.api.selcloud.ru/public-net/",
	}

	client, err := publicnetapi.NewPublicNetAPIClient(cfg)
	if err != nil {
		log.Fatal(err)
	}

	ports, err := client.ListPorts(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}

	for _, port := range ports {
		fmt.Printf("Port %s: %s\n", port.ID, port.IPAddress)
	}
}
```

## Supported operations

* **Port** — List, Get, Create, Update, Delete
* **Project quotas** — Get
