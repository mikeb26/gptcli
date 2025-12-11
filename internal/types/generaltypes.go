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

//go:generate mockgen --build_flags=--mod=mod -destination=openai_client_mock.go -package=$GOPACKAGE github.com/mikeb26/gptcli/internal/types GptCliAIClient
type GptCliAIClient interface {
	CreateChatCompletion(context.Context, []*GptCliMessage) (*GptCliMessage, error)
	StreamChatCompletion(context.Context, []*GptCliMessage) (*schema.StreamReader[*GptCliMessage], error)
	SetReasoning(laclopenai.ReasoningEffortLevel)
}
