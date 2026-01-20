/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package llmclient

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	ub "github.com/cloudwego/eino/utils/callbacks"
	"github.com/mikeb26/gptcli/internal/types"
)

type statusModelCallbacks struct {
	client *EINOAIClient
}

func (h *statusModelCallbacks) OnStart(
	ctx context.Context,
	info *callbacks.RunInfo,
	input *model.CallbackInput,
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentModel,
		DisplayText:  "",
		Phase:        types.ProgressPhaseStart,
		Time:         time.Now(),
	})
	return ctx
}

func (h *statusModelCallbacks) OnEnd(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *model.CallbackOutput,
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentModel,
		DisplayText:  "",
		Phase:        types.ProgressPhaseEnd,
		Time:         time.Now(),
	})
	return ctx
}

func (h *statusModelCallbacks) OnEndWithStreamOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *schema.StreamReader[*model.CallbackOutput],
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentModel,
		DisplayText:  "",
		Phase:        types.ProgressPhaseStreamingResp,
		Time:         time.Now(),
	})
	return ctx
}

type statusToolCallbacks struct {
	client *EINOAIClient
}

func (h *statusToolCallbacks) OnStart(
	ctx context.Context,
	info *callbacks.RunInfo,
	input *tool.CallbackInput,
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	name := getRunName("tool", info)
	args := "<nil>"
	if input != nil {
		args = summarizeText(input.ArgumentsInJSON)
	}

	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentTool,
		DisplayText:  fmt.Sprintf("->%v(%v)", name, args),
		Phase:        types.ProgressPhaseStart,
		Time:         time.Now(),
	})
	return ctx
}

func (h *statusToolCallbacks) OnEnd(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *tool.CallbackOutput,
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	name := getRunName("tool", info)
	resp := "<nil>"
	if output != nil {
		resp = summarizeText(output.Response)
	}

	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentTool,
		DisplayText:  fmt.Sprintf("<-%v: %v", name, resp),
		Phase:        types.ProgressPhaseEnd,
		Time:         time.Now(),
	})
	return ctx
}

func (h *statusToolCallbacks) OnEndWithStreamOutput(
	ctx context.Context,
	info *callbacks.RunInfo,
	output *schema.StreamReader[*tool.CallbackOutput],
) context.Context {
	id := GetInvocationID(ctx)
	if id == "" || h.client == nil {
		return ctx
	}

	name := getRunName("tool", info)
	h.client.publishProgress(id, types.ProgressEvent{
		InvocationID: id,
		Component:    types.ProgressComponentTool,
		DisplayText:  fmt.Sprintf("<-%v: <stream>", name),
		Phase:        types.ProgressPhaseStreamingResp,
		Time:         time.Now(),
	})
	return ctx
}

func newStatusModelHandler(client *EINOAIClient) *ub.ModelCallbackHandler {
	cb := &statusModelCallbacks{client: client}
	return &ub.ModelCallbackHandler{
		OnStart:               cb.OnStart,
		OnEnd:                 cb.OnEnd,
		OnEndWithStreamOutput: cb.OnEndWithStreamOutput,
	}
}

func newStatusToolHandler(client *EINOAIClient) *ub.ToolCallbackHandler {
	cb := &statusToolCallbacks{client: client}
	return &ub.ToolCallbackHandler{
		OnStart:               cb.OnStart,
		OnEnd:                 cb.OnEnd,
		OnEndWithStreamOutput: cb.OnEndWithStreamOutput,
	}
}

func newStatusCallbackHandlers(client *EINOAIClient) callbacks.Handler {
	helper := ub.NewHandlerHelper().
		ChatModel(newStatusModelHandler(client)).
		Tool(newStatusToolHandler(client))

	return helper.Handler()
}
