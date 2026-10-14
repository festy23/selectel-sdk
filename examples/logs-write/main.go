package main

import (
	"context"
	"github.com/festy23/selectel-sdk/logs"
	"github.com/festy23/selectel-sdk/types"
)

// write is an opt-in helper: the caller supplies an authorized scoped client,
// exact existing stream ref, timestamp and protected event. It runs no setup.
func write(ctx context.Context, client *logs.Client, stream types.ResourceRef, timestamp types.Integer, message types.ProtectedValue) (types.Result[logs.PutLogEventsResponse], error) {
	return client.PutEvents(ctx, stream, []logs.InputLogEvent{{Timestamp: &timestamp, Message: &message}}, types.CallOptions{})
}
func main() {}
