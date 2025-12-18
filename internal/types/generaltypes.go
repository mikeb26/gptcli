/* Copyright © 2024-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

import (
	"context"

	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// wrap eino with our own types/interfaces in order to enable the possibility
// of switching frameworks easily in the future

type GptCliMessage schema.Message
type GptCliTool tool.BaseTool
type GptCliRole schema.RoleType

const GptCliMessageRoleSystem = schema.System
const GptCliMessageRoleAssistant = schema.Assistant
const GptCliMessageRoleUser = schema.User

// StreamResult is returned by StreamChatCompletion to provide both the
// streaming reader and a stable invocation ID that can be used by callers
// to correlate callback-driven progress updates.
type StreamResult struct {
	InvocationID string
	Stream       *schema.StreamReader[*GptCliMessage]
}

// NOTE: gomock/mockgen does not yet fully understand Go generics syntax such
// as *schema.StreamReader[*GptCliMessage], so we no longer auto-generate this
// mock via go:generate. The mock implementation in openai_client_mock.go is
// maintained by hand.
//
//go:generate echo "skipping gomock generation for GptCliAIClient; using hand-maintained mock in openai_client_mock.go"
type GptCliAIClient interface {
	CreateChatCompletion(context.Context, []*GptCliMessage) (*GptCliMessage, error)
	StreamChatCompletion(context.Context, []*GptCliMessage) (*StreamResult, error)
	SetReasoning(laclopenai.ReasoningEffortLevel)
	SubscribeProgress(string) chan ProgressEvent
	UnsubscribeProgress(chan ProgressEvent, string)
}
