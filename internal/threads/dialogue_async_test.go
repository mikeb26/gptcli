/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/golang/mock/gomock"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/llmclient"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

type noopApprover struct{}

func (n noopApprover) AskApproval(ctx context.Context, req am.ApprovalRequest) (am.ApprovalDecision, error) {
	return am.ApprovalDecision{Allowed: true}, nil
}

func TestChatOnceAsyncStreamsAndFinalizes(t *testing.T) {
	dir := t.TempDir()
	grp := NewThreadGroup("", dir)
	assert.NoError(t, grp.NewThread("t1"))
	threads := grp.Threads()
	if !assert.Len(t, threads, 1) {
		return
	}
	thrImpl := threads[0].(*thread)

	ctxBase := context.Background()
	ctx, invocationID := llmclient.EnsureInvocationID(ctxBase)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := types.NewMockAIClient(ctrl)

	progressCh := make(chan types.ProgressEvent, 1)
	mockClient.EXPECT().SubscribeProgress(invocationID).Return(progressCh).Times(1)
	mockClient.EXPECT().UnsubscribeProgress(progressCh, invocationID).Times(1)

	sr, sw := schema.Pipe[*types.ThreadMessage](8)
	// Simulate streaming chunks.
	sw.Send(&types.ThreadMessage{Role: types.LlmRoleAssistant, Content: "Hello "}, nil)
	sw.Send(&types.ThreadMessage{Role: types.LlmRoleAssistant, Content: "world"}, nil)
	sw.Close()

	// Expect the stream call. Validate we at least get system+user messages.
	mockClient.EXPECT().StreamChatCompletion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, msgs []*types.ThreadMessage) (*types.StreamResult, error) {
			if assert.Len(t, msgs, 2) {
				assert.Equal(t, types.LlmRoleSystem, msgs[0].Role)
				assert.Equal(t, prompts.SystemMsg, msgs[0].Content)
				assert.Equal(t, types.LlmRoleUser, msgs[1].Role)
				assert.Equal(t, "hi", msgs[1].Content)
			}
			return &types.StreamResult{InvocationID: invocationID, Stream: sr}, nil
		},
	).Times(1)

	ictx := types.InternalContext{LlmBaseApprover: noopApprover{}}
	// Inject a mock client into the active thread so ChatOnceAsync uses it.
	thrImpl.llmClient = mockClient
	state, err := thrImpl.ChatOnceAsync(ctx, ictx, "hi", false)
	assert.NoError(t, err)
	if assert.NotNil(t, state) {
		assert.Equal(t, invocationID, state.InvocationID)
	}

	// Poll the accumulated content while the worker streams.
	assert.Eventually(t, func() bool {
		return state.ContentSoFar() == "Hello world"
	}, 2*time.Second, 5*time.Millisecond)

	res := <-state.Result
	assert.NoError(t, res.Err)
	if assert.NotNil(t, res.Reply) {
		assert.Equal(t, "Hello world", res.Reply.Content)
	}

	<-state.Done

	// Thread is finalized in-memory.
	thr := grp.Threads()[0]
	assert.Equal(t, ThreadStateIdle, thr.State())
	d := thr.Dialogue()
	if assert.Len(t, d, 3) {
		assert.Equal(t, types.LlmRoleSystem, d[0].Role)
		assert.Equal(t, types.LlmRoleUser, d[1].Role)
		assert.Equal(t, types.LlmRoleAssistant, d[2].Role)
		assert.Equal(t, "Hello world", d[2].Content)
	}
}

func TestChatOnceAsyncPropagatesStreamError(t *testing.T) {
	dir := t.TempDir()
	grp := NewThreadGroup("", dir)
	assert.NoError(t, grp.NewThread("t1"))
	threads := grp.Threads()
	if !assert.Len(t, threads, 1) {
		return
	}
	thrImpl := threads[0].(*thread)

	ctxBase := context.Background()
	ctx, invocationID := llmclient.EnsureInvocationID(ctxBase)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := types.NewMockAIClient(ctrl)

	progressCh := make(chan types.ProgressEvent, 1)
	mockClient.EXPECT().SubscribeProgress(invocationID).Return(progressCh).Times(1)
	mockClient.EXPECT().UnsubscribeProgress(progressCh, invocationID).Times(1)

	sr, sw := schema.Pipe[*types.ThreadMessage](8)
	sw.Send(&types.ThreadMessage{Role: types.LlmRoleAssistant, Content: "partial"}, nil)
	sw.Send(nil, io.ErrUnexpectedEOF)
	sw.Close()

	mockClient.EXPECT().StreamChatCompletion(gomock.Any(), gomock.Any()).Return(
		&types.StreamResult{InvocationID: invocationID, Stream: sr}, nil,
	).Times(1)

	ictx := types.InternalContext{LlmBaseApprover: noopApprover{}}
	// Inject a mock client into the active thread so ChatOnceAsync uses it.
	thrImpl.llmClient = mockClient
	state, err := thrImpl.ChatOnceAsync(ctx, ictx, "hi", false)
	assert.NoError(t, err)

	// Wait until we have at least the partial content.
	assert.Eventually(t, func() bool {
		return state.ContentSoFar() == "partial"
	}, 2*time.Second, 5*time.Millisecond)

	res := <-state.Result
	assert.Error(t, res.Err)
	<-state.Done

	// Thread should not have been finalized with an assistant reply.
	thr := grp.Threads()[0]
	assert.Len(t, thr.Dialogue(), 1) // still only system message
}
