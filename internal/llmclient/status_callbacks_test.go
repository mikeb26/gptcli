/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package llmclient

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestStatusCallbacks_ModelStartEnd(t *testing.T) {
	client := &GptCliEINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}
	invID := "inv-1"

	ch := client.SubscribeProgress(invID)
	if !assert.NotNil(t, ch) {
		return
	}
	defer client.UnsubscribeProgress(ch, invID)

	ctx := context.WithValue(context.Background(), invocationIDKey{}, invID)

	modelCB := &statusModelCallbacks{client: client}
	modelCB.OnStart(ctx, &callbacks.RunInfo{Name: "m"}, &model.CallbackInput{})
	modelCB.OnEnd(ctx, &callbacks.RunInfo{Name: "m"}, &model.CallbackOutput{})

	// We should have at least one progress event saved.
	client.currentMu.RLock()
	cur, ok := client.current[invID]
	client.currentMu.RUnlock()
	assert.True(t, ok)
	assert.Equal(t, types.ProgressComponentModel, cur.Component)
}

func TestStatusCallbacks_ToolStartEnd(t *testing.T) {
	client := &GptCliEINOAIClient{
		subs:    make(map[string][]chan types.ProgressEvent),
		current: make(map[string]types.ProgressEvent),
	}
	invID := "inv-1"

	ch := client.SubscribeProgress(invID)
	if !assert.NotNil(t, ch) {
		return
	}
	defer client.UnsubscribeProgress(ch, invID)

	ctx := context.WithValue(context.Background(), invocationIDKey{}, invID)

	toolCB := &statusToolCallbacks{client: client}
	toolCB.OnStart(ctx, &callbacks.RunInfo{Name: "t"}, &einotool.CallbackInput{ArgumentsInJSON: "{\"a\":1}"})
	toolCB.OnEnd(ctx, &callbacks.RunInfo{Name: "t"}, &einotool.CallbackOutput{Response: "{\"b\":2}"})

	client.currentMu.RLock()
	cur, ok := client.current[invID]
	client.currentMu.RUnlock()
	assert.True(t, ok)
	assert.Equal(t, types.ProgressComponentTool, cur.Component)
	assert.Contains(t, cur.DisplayText, "t")
}
