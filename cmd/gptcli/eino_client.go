/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"

	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
	"github.com/mikeb26/gptcli/internal"
)

type GptCliEINOAIClient struct {
	reactAgent *react.Agent
}

func NewEINOAIClient(ctx context.Context, input *bufio.Reader, apiKey string,
	model string, depth int) internal.GptCliAIClient {
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  model,
		APIKey: apiKey,
	})
	if err != nil {
		panic(err)
	}
	tools := defineTools(ctx, input, apiKey, model, depth)
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
