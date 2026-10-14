package main

import (
	"context"
	"github.com/festy23/selectel-sdk/metrics"
	"github.com/festy23/selectel-sdk/types"
)

// read needs a caller-authorized project/location client. It never prints raw
// labels, arbitrary string samples, warning text or credentials.
func read(ctx context.Context, client *metrics.Client) (types.Result[metrics.QueryData], error) {
	return client.Query(ctx, metrics.QueryRequest{Namespace: "compute", PromQL: "up"})
}
func main() {}
