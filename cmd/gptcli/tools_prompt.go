/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal"
)

type PromptRunTool struct {
	ctx    context.Context
	client internal.GptCliAIClient
	input  *bufio.Reader
	depth  int
}

type PromptRunReq struct {
	Dialogue []*internal.GptCliMessage `json:"dialogue" jsonschema:"description=The dialogue to send to the LLM"`
}

type PrompRunResp struct {
	Error   string                 `json:"error" jsonschema:"description=The error status of the prompt run call"`
	Message internal.GptCliMessage `json:"error" jsonschema:"description=The message returned by the LLM"`
}

func (t PromptRunTool) GetOp() ToolCallOp {
	return PromptRun
}

func (t PromptRunTool) RequiresUserApproval() bool {
	return false
}

func NewPromptRunTool(ctxIn context.Context, inputIn *bufio.Reader,
	apiKey string, model string, depthIn int) internal.GptCliTool {
	t := &PromptRunTool{
		ctx:    ctxIn,
		input:  inputIn,
		depth:  depthIn,
		client: NewEINOAIClient(ctxIn, inputIn, apiKey, model, depthIn+1),
	}

	return t.Define()
}

func (t PromptRunTool) Define() internal.GptCliTool {
	const NonLeafDesc = "Query the OpenAI model with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks (which themselves could be further subtasked). It has access to the same set of tools gptcli provides."
	const LeafDesc = "Query the OpenAI model with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks. It has access to the same set of tools gptcli provides (except this one)."

	desc := NonLeafDesc
	if t.depth >= MaxDepth {
		desc = LeafDesc
	}
	ret, err := utils.InferTool(string(t.GetOp()), desc, t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t PromptRunTool) Invoke(ctx context.Context,
	req *PromptRunReq) (*PrompRunResp, error) {

	ret := &PrompRunResp{}

	err := getUserApproval(t.input, t, req.String())
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	fmt.Printf("gptcli: subtask depth %v processing...\n", t.depth)
	msg, err := t.client.CreateChatCompletion(t.ctx, req.Dialogue)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}
	ret.Message = *msg

	return ret, nil
}

func (req *PromptRunReq) String() string {
	var sb strings.Builder

	sb.WriteString("Dialogue:[")
	for _, msg := range req.Dialogue {
		sb.WriteString(fmt.Sprintf("{Role:%v,Msg:%v}", string(msg.Role),
			msg.Content))
	}
	sb.WriteString("]")

	return sb.String()
}
