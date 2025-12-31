/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package threads

import (
	"context"
	"errors"
	"io"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/llmclient"
	"github.com/mikeb26/gptcli/internal/types"
)

// RunningThreadChunk represents one incremental streamed message chunk or an
// error encountered while streaming.
//
// When Err is non-nil, Msg may be nil.
//
// Stream completion is indicated by the Chunk channel closing; errors are
// surfaced via Err and the Result channel.
type RunningThreadChunk struct {
	Msg *types.GptCliMessage
	Err error
}

// RunningThreadStart is sent once, when the request has been prepared and the
// streaming reader has been established (or the request failed before that).
type RunningThreadStart struct {
	Prepared *PreparedChat
	Err      error
}

// RunningThreadResult is sent once, when the request has completed (success or
// failure).
type RunningThreadResult struct {
	Prepared *PreparedChat
	Reply    *types.GptCliMessage
	Err      error
}

// RunningThreadState captures all asynchronous state for a single in-flight chat
// request.
//
// This struct is intended to be selected on by UI layers so they can render
// progress, serve proxy UI requests, and incrementally display streaming output
// without managing worker goroutines themselves.
type RunningThreadState struct {
	Prompt       string
	InvocationID string

	Thread   *GptCliThread
	Prepared *PreparedChat

	Progress         <-chan types.ProgressEvent
	Start            <-chan RunningThreadStart
	Chunk            <-chan RunningThreadChunk
	ApprovalRequests <-chan am.AsyncApprovalRequest

	Result <-chan RunningThreadResult
	Done   <-chan struct{}

	Cancel context.CancelFunc
}

// Stop cancels the in-flight request (best-effort).
func (s *RunningThreadState) Stop() {
	if s == nil || s.Cancel == nil {
		return
	}
	s.Cancel()
}

// ChatOnceAsync is the fully-asynchronous analogue of
// ChatOnce / ChatOnceStream.
//
// It returns immediately with a RunningThreadState that exposes channels for:
//   - progress events
//   - prepared/start notification
//   - streamed chunks/errors
//   - proxy UI requests
//   - final result
//   - completion
//
// The worker goroutine fully manages the request lifecycle, including
// finalizing and persisting the thread upon success.
func (thrGrp *GptCliThreadGroup) ChatOnceAsync(
	ctx context.Context, llmClient types.GptCliAIClient, prompt string,
	summarizePrior bool,
	asyncApprover *am.AsyncApprover,
) (*RunningThreadState, error) {
	// Record the current thread immediately so that the lifetime of this run is
	// independent of any subsequent changes to the thread group's notion of
	// "current thread".
	thread, err := thrGrp.getCurrentThread()
	if err != nil {
		return nil, err
	}
	thread.SetState(GptCliThreadStateRunning)

	// Seed an invocation ID up-front so the UI can subscribe to progress events
	// before the agent begins executing.
	ctx, invocationID := llmclient.EnsureInvocationID(ctx)
	ctx, cancel := context.WithCancel(ctx)

	progressCh := llmClient.SubscribeProgress(invocationID)
	startCh := make(chan RunningThreadStart, 1)
	chunkCh := make(chan RunningThreadChunk, 16)
	resultCh := make(chan RunningThreadResult, 1)
	doneCh := make(chan struct{})

	state := &RunningThreadState{
		Prompt:           prompt,
		InvocationID:     invocationID,
		Thread:           thread,
		Progress:         progressCh,
		Start:            startCh,
		Chunk:            chunkCh,
		ApprovalRequests: asyncApprover.Requests,
		Result:           resultCh,
		Done:             doneCh,
		Cancel:           cancel,
	}

	go runChatOnceAsync(
		thrGrp, ctx, llmClient, thread, prompt, summarizePrior,
		invocationID, progressCh,
		state, startCh, chunkCh, resultCh, doneCh,
	)

	return state, nil
}

func runChatOnceAsync(
	thrGrp *GptCliThreadGroup,
	ctx context.Context,
	llmClient types.GptCliAIClient,
	thread *GptCliThread,
	prompt string,
	summarizePrior bool,
	invocationID string,
	progressCh chan types.ProgressEvent,
	state *RunningThreadState,
	startCh chan<- RunningThreadStart,
	chunkCh chan<- RunningThreadChunk,
	resultCh chan<- RunningThreadResult,
	doneCh chan<- struct{},
) {
	defer close(doneCh)
	defer close(chunkCh)
	defer close(resultCh)
	defer llmClient.UnsubscribeProgress(progressCh, invocationID)

	prep, stream, err := thrGrp.chatOnceStreamInThread(ctx, llmClient, thread, prompt, summarizePrior)
	if err != nil {
		startCh <- RunningThreadStart{Prepared: nil, Err: err}
		close(startCh)
		resultCh <- RunningThreadResult{Prepared: nil, Reply: nil, Err: err}
		thread.SetState(GptCliThreadStateIdle)
		return
	}
	if prep == nil || stream == nil {
		err := errors.New("nil stream result")
		startCh <- RunningThreadStart{Prepared: nil, Err: err}
		close(startCh)
		resultCh <- RunningThreadResult{Prepared: nil, Reply: nil, Err: err}
		thread.SetState(GptCliThreadStateIdle)
		return
	}
	state.Prepared = prep

	startCh <- RunningThreadStart{Prepared: prep, Err: nil}
	close(startCh)

	defer stream.Close()
	go closeStreamOnCancel(ctx, stream)

	var buffer strings.Builder
	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			trySendChunk(ctx, chunkCh, RunningThreadChunk{Msg: nil, Err: recvErr})
			resultCh <- RunningThreadResult{Prepared: prep, Reply: nil, Err: recvErr}
			thread.SetState(GptCliThreadStateIdle)
			return
		}
		if msg == nil {
			continue
		}
		buffer.WriteString(msg.Content)
		trySendChunk(ctx, chunkCh, RunningThreadChunk{Msg: msg, Err: nil})
	}

	replyMsg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleAssistant,
		Content: buffer.String(),
	}
	if err := thrGrp.finalizeChatOnce(prep, replyMsg); err != nil {
		resultCh <- RunningThreadResult{Prepared: prep, Reply: nil, Err: err}
		return
	}

	resultCh <- RunningThreadResult{Prepared: prep, Reply: replyMsg, Err: nil}
}

func closeStreamOnCancel(ctx context.Context, stream *schema.StreamReader[*types.GptCliMessage]) {
	if ctx == nil || stream == nil {
		return
	}
	<-ctx.Done()
	stream.Close()
}

func trySendChunk(ctx context.Context, ch chan<- RunningThreadChunk, ev RunningThreadChunk) {
	select {
	case <-ctx.Done():
		return
	case ch <- ev:
		return
	}
}
