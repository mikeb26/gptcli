/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type PromptRunTool struct {
	ctx        context.Context
	client     types.GptCliAIClient
	approvalUI ToolApprovalUI
	depth      int
}

type PromptRunReq struct {
	Dialogue []*types.GptCliMessage `json:"dialogue" jsonschema:"description=The dialogue to send to the LLM"`
}

type PrompRunResp struct {
	Error   string              `json:"error" jsonschema:"description=The error status of the prompt run call"`
	Message types.GptCliMessage `json:"error" jsonschema:"description=The message returned by the LLM"`
}

func (t PromptRunTool) GetOp() types.ToolCallOp {
	return types.PromptRun
}

func (t PromptRunTool) RequiresUserApproval() bool {
	return false
}
func NewPromptRunTool(ctxIn context.Context, vendor string, approvalUI ToolApprovalUI,
	apiKey string, model string, depthIn int) types.GptCliTool {

	t := &PromptRunTool{
		ctx:        ctxIn,
		approvalUI: approvalUI,
		depth:      depthIn,
		client: NewEINOClient(ctxIn, vendor, approvalUI.GetUI(), apiKey,
			model, depthIn+1),
	}

	return t.Define()
}

func (t PromptRunTool) Define() types.GptCliTool {
	const NonLeafDesc = "Query the LLM with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks (which themselves could be further subtasked). It has access to the same set of tools gptcli provides."
	const LeafDesc = "Query the LLM with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks. It has access to the same set of tools gptcli provides (except this one)."

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

	err := getUserApproval(t.approvalUI, t, req.String())
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
