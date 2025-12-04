/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/gemini"
	"github.com/cloudwego/eino-ext/components/model/openai"
	laclopenai "github.com/cloudwego/eino-ext/libs/acl/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/tools"
	"github.com/mikeb26/gptcli/internal/types"
	"google.golang.org/genai"
)

type GptCliEINOAIClient struct {
	reactAgent      *react.Agent
	reasoningEffort laclopenai.ReasoningEffortLevel
}

func NewEINOClient(ctx context.Context, vendor string,
	ui types.GptCliUI, apiKey string, model string,
	depth int) types.GptCliAIClient {

	if vendor == "openai" {
		return newOpenAIEINOClient(ctx, vendor, ui, apiKey, model, depth)
	} else if vendor == "anthropic" {
		return newAnthropicEINOClient(ctx, vendor, ui, apiKey, model, depth)
	} else if vendor == "google" {
		return newGoogleEINOClient(ctx, vendor, ui, apiKey, model, depth)
	} // else

	panic("unsupported vendor")
	return nil
}

func newOpenAIEINOClient(ctx context.Context, vendor string,
	ui types.GptCliUI, apiKey string, model string,
	depth int) types.GptCliAIClient {

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, ui, apiKey, model, depth)
}

func newAnthropicEINOClient(ctx context.Context, vendor string,
	ui types.GptCliUI, apiKey string, model string,
	depth int) types.GptCliAIClient {

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, ui, apiKey, model, depth)
}

func newGoogleEINOClient(ctx context.Context, vendor string,
	ui types.GptCliUI, apiKey string, model string,
	depth int) types.GptCliAIClient {

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	chatModel, err := gemini.NewChatModel(ctx, &gemini.Config{
		Model:  model,
		Client: client,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, ui, apiKey, model, depth)
}

func newEINOClient(ctx context.Context, vendor string, chatModel model.ChatModel,
	ui types.GptCliUI, apiKey string, model string,
	depth int) types.GptCliAIClient {

	tools := defineTools(ctx, vendor, ui, apiKey, model, depth)
	baseTools := make([]tool.BaseTool, len(tools))
	for ii, _ := range tools {
		baseTools[ii] = tools[ii]
	}
	config := &react.AgentConfig{
		Model:   chatModel,
		MaxStep: 250,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: baseTools,
		},
	}

	client, err := react.NewAgent(ctx, config)
	if err != nil {
		panic(err)
	}

	return &GptCliEINOAIClient{
		reactAgent:      client,
		reasoningEffort: laclopenai.ReasoningEffortLevelMedium,
	}
}

func defineTools(ctx context.Context, vendor string, ui types.GptCliUI,
	apiKey string, model string, depth int) []types.GptCliTool {

	approvalUI := tools.NewApprovalUI(ui)
	tools := []types.GptCliTool{
		tools.NewRunCommandTool(approvalUI),
		tools.NewCreateFileTool(approvalUI),
		tools.NewAppendFileTool(approvalUI),
		tools.NewFilePatchTool(approvalUI),
		tools.NewReadFileTool(approvalUI),
		tools.NewDeleteFileTool(approvalUI),
		tools.NewPwdTool(approvalUI),
		tools.NewChdirTool(approvalUI),
		tools.NewEnvGetTool(approvalUI),
		tools.NewEnvSetTool(approvalUI),
		tools.NewRetrieveUrlTool(approvalUI),
		tools.NewRenderWebTool(approvalUI),
	}
	if depth <= internal.MaxDepth {
		tools = append(tools, newPromptRunTool(ctx, vendor, approvalUI, apiKey,
			model, depth))
	}

	return tools
}

func (client *GptCliEINOAIClient) SetReasoning(
	reasoningEffort laclopenai.ReasoningEffortLevel) {
	client.reasoningEffort = reasoningEffort
}

func (client GptCliEINOAIClient) CreateChatCompletion(ctx context.Context,
	dialogueIn []*types.GptCliMessage) (*types.GptCliMessage, error) {

	dialogue := make([]*schema.Message, len(dialogueIn))
	for ii, msg := range dialogueIn {
		dialogue[ii] = (*schema.Message)(msg)
	}

	modelOpt := laclopenai.WithReasoningEffort(client.reasoningEffort)
	composeOpt := compose.WithChatModelOption(modelOpt)
	agentOpt := agent.WithComposeOptions(composeOpt)

	msg, err := client.reactAgent.Generate(ctx, dialogue, agentOpt)
	return (*types.GptCliMessage)(msg), err
}
