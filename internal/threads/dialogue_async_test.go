/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"io"
	"os"
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
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "", grpDir)
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
	state, err := thrImpl.ChatOnceAsync(ctx, ictx, "hi", false, prompts.SystemMsg)
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
	// Product code currently persists the system message as part of the dialogue.
	if assert.Len(t, d, 2) {
		assert.Equal(t, types.LlmRoleUser, d[0].Role)
		assert.Equal(t, types.LlmRoleAssistant, d[1].Role)
		assert.Equal(t, "Hello world", d[1].Content)
	}
}

func TestChatOnceAsyncRequiresSystemMessage(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "", grpDir)
	assert.NoError(t, grp.NewThread("t1"))
	threads := grp.Threads()
	if !assert.Len(t, threads, 1) {
		return
	}
	thrImpl := threads[0].(*thread)

	ictx := types.InternalContext{LlmBaseApprover: noopApprover{}}
	state, err := thrImpl.ChatOnceAsync(context.Background(), ictx, "hi", false, "")
	assert.Error(t, err)
	assert.Nil(t, state)
}

func TestChatOnceAsyncPropagatesStreamError(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "", grpDir)
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
	state, err := thrImpl.ChatOnceAsync(ctx, ictx, "hi", false, prompts.SystemMsg)
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
	assert.Len(t, thr.Dialogue(), 0) // no persisted messages
}

func TestChatOnceAsyncDropsPersistedSystemMessageForBackwardsCompatibility(t *testing.T) {
	root := t.TempDir()
	setDir := root + "/set"
	grpDir := root + "/threads"
	assert.NoError(t, os.MkdirAll(setDir, 0o700))
	assert.NoError(t, os.MkdirAll(grpDir, 0o700))

	set := NewThreadGroupSet(setDir, nil)
	grp := newThreadGroup(set, "", grpDir)
	assert.NoError(t, grp.NewThread("t1"))
	threads := grp.Threads()
	if !assert.Len(t, threads, 1) {
		return
	}
	thrImpl := threads[0].(*thread)

	// Simulate an old persisted thread that kept the system message in Dialogue.
	thrImpl.persisted.Dialogue = []*types.ThreadMessage{
		{Role: types.LlmRoleSystem, Content: "OLD SYSTEM"},
		{Role: types.LlmRoleUser, Content: "prior user"},
		{Role: types.LlmRoleAssistant, Content: "prior assistant"},
	}

	ctxBase := context.Background()
	ctx, invocationID := llmclient.EnsureInvocationID(ctxBase)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := types.NewMockAIClient(ctrl)

	progressCh := make(chan types.ProgressEvent, 1)
	mockClient.EXPECT().SubscribeProgress(invocationID).Return(progressCh).Times(1)
	mockClient.EXPECT().UnsubscribeProgress(progressCh, invocationID).Times(1)

	sr, sw := schema.Pipe[*types.ThreadMessage](8)
	sw.Send(&types.ThreadMessage{Role: types.LlmRoleAssistant, Content: "ok"}, nil)
	sw.Close()

	mockClient.EXPECT().StreamChatCompletion(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, msgs []*types.ThreadMessage) (*types.StreamResult, error) {
			if assert.GreaterOrEqual(t, len(msgs), 1) {
				assert.Equal(t, types.LlmRoleSystem, msgs[0].Role)
				assert.Equal(t, prompts.SystemMsg, msgs[0].Content)
			}
			for _, m := range msgs {
				if m == nil {
					continue
				}
				assert.False(t, m.Role == types.LlmRoleSystem && m.Content == "OLD SYSTEM")
			}
			return &types.StreamResult{InvocationID: invocationID, Stream: sr}, nil
		},
	).Times(1)

	ictx := types.InternalContext{LlmBaseApprover: noopApprover{}}
	thrImpl.llmClient = mockClient
	state, err := thrImpl.ChatOnceAsync(ctx, ictx, "hi", false, prompts.SystemMsg)
	assert.NoError(t, err)
	res := <-state.Result
	assert.NoError(t, res.Err)
	<-state.Done

	d := thrImpl.Dialogue()
	// Persisted dialogue should not include the *old* persisted system message,
	// but may include the current system message.
	for _, m := range d {
		if m == nil {
			continue
		}
		assert.False(t, m.Role == types.LlmRoleSystem && m.Content == "OLD SYSTEM")
	}
}
