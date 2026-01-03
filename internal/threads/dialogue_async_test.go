package threads

import (
	"context"
	"io"
	"strings"
	"testing"

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
	_, err := grp.ActivateThread(1)
	assert.NoError(t, err)

	ctxBase := context.Background()
	ctx, invocationID := llmclient.EnsureInvocationID(ctxBase)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := types.NewMockGptCliAIClient(ctrl)

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

	asyncApprover := NewAsyncApprover(noopApprover{})
	state, err := grp.ChatOnceAsync(ctx, mockClient, "hi", false, asyncApprover)
	assert.NoError(t, err)
	if assert.NotNil(t, state) {
		assert.Equal(t, invocationID, state.InvocationID)
	}

	start := <-state.Start
	assert.NoError(t, start.Err)
	assert.NotNil(t, start.Prepared)

	var got strings.Builder
	for ce := range state.Chunk {
		if ce.Err != nil {
			assert.FailNow(t, "unexpected stream error", ce.Err.Error())
		}
		if ce.Msg != nil {
			got.WriteString(ce.Msg.Content)
		}
	}
	assert.Equal(t, "Hello world", got.String())

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
	_, err := grp.ActivateThread(1)
	assert.NoError(t, err)

	ctxBase := context.Background()
	ctx, invocationID := llmclient.EnsureInvocationID(ctxBase)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockClient := types.NewMockGptCliAIClient(ctrl)

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

	asyncApprover := NewAsyncApprover(noopApprover{})
	state, err := grp.ChatOnceAsync(ctx, mockClient, "hi", false, asyncApprover)
	assert.NoError(t, err)
	start := <-state.Start
	assert.NoError(t, start.Err)

	// Drain chunks; we expect at least one error chunk.
	gotErr := false
	for ce := range state.Chunk {
		if ce.Err != nil {
			gotErr = true
		}
	}
	assert.True(t, gotErr)

	res := <-state.Result
	assert.Error(t, res.Err)
	<-state.Done

	// Thread should not have been finalized with an assistant reply.
	thr := grp.Threads()[0]
	assert.Len(t, thr.Dialogue(), 1) // still only system message
}
