// Package aichat demonstrates an explicitly invoked, potentially billable model
// request. This package has no main/init call and performs no cloud work on load.
package aichat

import (
	"context"
	"io"

	sdk "github.com/festy23/selectel-sdk"
	"github.com/festy23/selectel-sdk/ai"
	"github.com/festy23/selectel-sdk/types"
)

// RouterChat requires an already configured caller-owned Client and writer.
// The caller retains the receipt on every error and must never automatically
// repeat an unknown outcome. CopyTo explicitly reveals content to the writer.
func RouterChat(ctx context.Context, client *sdk.Client, model string, prompt types.ProtectedValue, writer io.Writer) (*types.Receipt, error) {
	result, err := client.Account().AIRouter().Chat(ctx, ai.ChatRequest{Model: model, Messages: []ai.Message{{Role: "user", Content: prompt}}})
	if err != nil {
		return result.Receipt, err
	}
	for _, choice := range result.Data.Choices {
		if choice.Message.Content != nil {
			if _, err := choice.Message.Content.CopyTo(writer); err != nil {
				return result.Receipt, err
			}
		}
	}
	return result.Receipt, nil
}
