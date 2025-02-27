/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type PromptRunTool struct {
	ctx    context.Context
	client *openai.Client
	input  *bufio.Reader
}

func NewPromptRunTool(ctxIn context.Context, clientIn *openai.Client,
	inputIn *bufio.Reader) PromptRunTool {

	return PromptRunTool{
		ctx:    ctxIn,
		client: clientIn,
		input:  inputIn,
	}
}

func (t PromptRunTool) GetOp() ToolCallOp {
	return PromptRun
}

func (t PromptRunTool) RequiresUserApproval() bool {
	return false
}

func (PromptRunTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"system_prompt": {
				Type:        jsonschema.String,
				Description: "The system prompt to initiate an LLM dialogue",
			},
			"user_prompt": {
				Type:        jsonschema.String,
				Description: "The user prompt to initiate an LLM dialogue",
			},
		},
		Required: []string{"system_prompt", "user_prompt"},
	}
	f := openai.FunctionDefinition{
		Name:        string(PromptRun),
		Description: "Query the OpenAI model with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks (which themselves could be further subtasked). It has access to the same set of tools gptcli provides.",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (prt PromptRunTool) Invoke(args map[string]any) (string, error) {
	sysprompt, ok := args["system_prompt"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'system_prompt' arg")
	}
	usrprompt, ok := args["user_prompt"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'user_prompt' arg")
	}

	// @todo refactor and deduplicate w/ interactiveThreadWork()
	dialogue := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: sysprompt},
		{Role: openai.ChatMessageRoleUser, Content: usrprompt},
	}
	tools := defineTools(prt.ctx, prt.client, prt.input)

	var err error
	var resp openai.ChatCompletionResponse
	var msg openai.ChatCompletionMessage
	toolsCompleted := false
	for toolsCompleted == false {

		toolsCompleted = true

		fmt.Printf("gptcli: subtask processing...\n")
		resp, err = prt.client.CreateChatCompletion(prt.ctx,
			openai.ChatCompletionRequest{
				Model:           openai.O3Mini,
				Messages:        dialogue,
				Tools:           tools,
				ReasoningEffort: "high",
			},
		)
		if err != nil {
			return "", err
		}

		if len(resp.Choices) != 1 {
			return "", fmt.Errorf("gptcli: BUG: Expected 1 response, got %v",
				len(resp.Choices))
		}

		msg = resp.Choices[0].Message
		dialogue = append(dialogue, msg)

		for _, tc := range msg.ToolCalls {
			toolsCompleted = false
			var toolMsg openai.ChatCompletionMessage
			// @todo need a way to disambiguate errors that represent
			// problems in gptcli vs. tool runtime errors that should be
			// returned to openai via chat completion. for now return all.
			toolMsg, _ = processToolCall(tc, prt.input)
			dialogue = append(dialogue, toolMsg)
		}

		if toolsCompleted && msg.Content == "" {
			msg = openai.ChatCompletionMessage{
				Role:    openai.ChatMessageRoleSystem,
				Content: "You sent an empty response to the last user message; can you please respond?",
			}
			dialogue = append(dialogue, msg)
			toolsCompleted = false
		}
	}

	return msg.Content, err
}
