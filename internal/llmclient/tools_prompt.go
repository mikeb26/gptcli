/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/tools"
	"github.com/mikeb26/gptcli/internal/types"
)

// defined here due to recursion with newEINOClient()

type PromptRunTool struct {
	ctx         context.Context
	client      types.GptCliAIClient
	approvalUI  tools.ToolApprovalUI
	policyStore am.ApprovalPolicyStore
	depth       int
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
func newPromptRunTool(ctxIn context.Context, vendor string,
	approvalUI tools.ToolApprovalUI, apiKey string, model string, depthIn int,
	policyStore am.ApprovalPolicyStore) types.GptCliTool {

	t := &PromptRunTool{
		ctx:        ctxIn,
		approvalUI: approvalUI,
		depth:      depthIn,
		client: NewEINOClient(ctxIn, vendor, approvalUI.GetUI(), apiKey,
			model, depthIn+1, policyStore),
	}

	return t.Define()
}

func (t PromptRunTool) Define() types.GptCliTool {
	const NonLeafDesc = "Query the LLM with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks (which themselves could be further subtasked). It has access to the same set of tools gptcli provides."
	const LeafDesc = "Query the LLM with the provided system and user prompts. This function is useful for managing limited sized LLM context windows. Bigger picture tasks can be broken down into smaller more focused tasks. It has access to the same set of tools gptcli provides (except this one)."

	desc := NonLeafDesc
	if t.depth >= internal.MaxDepth {
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

	err := tools.GetUserApproval(ctx, t.approvalUI, t, req.String())
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	fmt.Printf("gptcli: subtask depth %v processing...\n", t.depth)

	// Always use the per-invocation context so that cancellation, deadlines,
	// and correlation IDs propagate correctly into nested LLM calls.
	msg, err := t.client.CreateChatCompletion(ctx, req.Dialogue)
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
