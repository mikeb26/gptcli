/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"

	"github.com/cloudwego/eino-ext/components/model/claude"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/mikeb26/gptcli/internal"
)

type GptCliEINOAIClient struct {
	reactAgent *react.Agent
}

func NewEINOClient(ctx context.Context, vendor string,
	input *bufio.Reader, apiKey string, model string,
	depth int) internal.GptCliAIClient {

	if vendor == "openai" {
		return newOpenAIEINOClient(ctx, vendor, input, apiKey, model, depth)
	} else if vendor == "anthropic" {
		return newAnthropicEINOClient(ctx, vendor, input, apiKey, model, depth)
	} // else

	panic("unsupported vendor")
	return nil
}

func newOpenAIEINOClient(ctx context.Context, vendor string,
	input *bufio.Reader, apiKey string, model string,
	depth int) internal.GptCliAIClient {

	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, input, apiKey, model, depth)
}

func newAnthropicEINOClient(ctx context.Context, vendor string,
	input *bufio.Reader, apiKey string, model string,
	depth int) internal.GptCliAIClient {

	chatModel, err := claude.NewChatModel(ctx, &claude.Config{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}

	return newEINOClient(ctx, vendor, chatModel, input, apiKey, model, depth)
}

func newEINOClient(ctx context.Context, vendor string, chatModel model.ChatModel,
	input *bufio.Reader, apiKey string, model string,
	depth int) internal.GptCliAIClient {

	tools := defineTools(ctx, vendor, input, apiKey, model, depth)
	baseTools := make([]tool.BaseTool, len(tools))
	for ii, _ := range tools {
		baseTools[ii] = tools[ii]
	}
	config := &react.AgentConfig{
		Model:   chatModel,
		MaxStep: 25,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: baseTools,
		},
	}

	client, err := react.NewAgent(ctx, config)
	if err != nil {
		panic(err)
	}

	return GptCliEINOAIClient{reactAgent: client}
}

func (client GptCliEINOAIClient) CreateChatCompletion(ctx context.Context,
	dialogueIn []*internal.GptCliMessage) (*internal.GptCliMessage, error) {

	dialogue := make([]*schema.Message, len(dialogueIn))
	for ii, msg := range dialogueIn {
		dialogue[ii] = (*schema.Message)(msg)
	}
	msg, err := client.reactAgent.Generate(ctx, dialogue)
	return (*internal.GptCliMessage)(msg), err
}
