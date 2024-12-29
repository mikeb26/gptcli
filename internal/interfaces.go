/* Copyright © 2023 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"context"

	"github.com/sashabaranov/go-openai"
)

//go:generate mockgen --build_flags=--mod=mod -destination=openai_client_mock.go -package=$GOPACKAGE github.com/mikeb26/gptcli/internal OpenAIClient
type OpenAIClient interface {
	CreateChatCompletion(ctx context.Context, request openai.ChatCompletionRequest) (response openai.ChatCompletionResponse, err error)
}
