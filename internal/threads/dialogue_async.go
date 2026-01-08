/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package threads

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/cloudwego/eino/schema"
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
	Msg *types.ThreadMessage
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
	Reply    *types.ThreadMessage
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

	Thread   Thread
	Prepared *PreparedChat

	Progress         <-chan types.ProgressEvent
	Start            <-chan RunningThreadStart
	Chunk            <-chan RunningThreadChunk
	ApprovalRequests <-chan AsyncApprovalRequest
	AsyncApprover    *AsyncApprover

	Result <-chan RunningThreadResult
	Done   <-chan struct{}

	Cancel context.CancelFunc

	mu sync.RWMutex
	// contentSoFarBuf accumulates the streamed content so far so that background
	// runs can continue even when no UI is consuming Chunk events.
	contentSoFarBuf []byte
}

// ContentSoFar returns the accumulated content so far.
//
// This is safe to call concurrently with the background streaming goroutine.
func (s *RunningThreadState) ContentSoFar() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return string(s.contentSoFarBuf)
}

func (s *RunningThreadState) appendContentSoFar(delta string) {
	s.mu.Lock()
	s.contentSoFarBuf = append(s.contentSoFarBuf, delta...)
	s.mu.Unlock()
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
func (thrGrp *ThreadGroup) ChatOnceAsync(
	ctx context.Context, ictx types.InternalContext, prompt string,
	summarizePrior bool,
) (*RunningThreadState, error) {
	// Record the current thread immediately so that the lifetime of this run is
	// independent of any subsequent changes to the thread group's notion of
	// "current thread".
	thread, err := thrGrp.setCurrentThreadRunning(ctx, ictx)
	if err != nil {
		return nil, err
	}

	// Seed an invocation ID up-front so the UI can subscribe to progress events
	// before the agent begins executing.
	ctx, invocationID := llmclient.EnsureInvocationID(ctx)
	ctx, cancel := context.WithCancel(ctx)
	ctx = WithThread(ctx, thread)

	progressCh := thread.llmClient.SubscribeProgress(invocationID)
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
		ApprovalRequests: thread.asyncApprover.Requests,
		AsyncApprover:    thread.asyncApprover,
		Result:           resultCh,
		Done:             doneCh,
		Cancel:           cancel,
	}

	thread.mu.Lock()
	thread.runState = state
	thread.mu.Unlock()

	go runChatOnceAsync(
		thrGrp, ctx, thread, prompt, summarizePrior,
		invocationID, progressCh,
		state, startCh, chunkCh, resultCh, doneCh,
	)

	return state, nil
}

func runChatOnceAsync(
	thrGrp *ThreadGroup,
	ctx context.Context,
	thread *thread,
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
	defer thread.llmClient.UnsubscribeProgress(progressCh, invocationID)

	prep, stream, err := thrGrp.chatOnceStreamInThread(ctx, thread, prompt, summarizePrior)
	if err != nil {
		startCh <- RunningThreadStart{Prepared: nil, Err: err}
		close(startCh)
		resultCh <- RunningThreadResult{Prepared: nil, Reply: nil, Err: err}
		return
	}
	if prep == nil || stream == nil {
		err := errors.New("nil stream result")
		startCh <- RunningThreadStart{Prepared: nil, Err: err}
		close(startCh)
		resultCh <- RunningThreadResult{Prepared: nil, Reply: nil, Err: err}
		return
	}
	state.Prepared = prep

	startCh <- RunningThreadStart{Prepared: prep, Err: nil}
	close(startCh)

	defer stream.Close()
	go closeStreamOnCancel(ctx, stream)

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			trySendChunk(ctx, chunkCh, RunningThreadChunk{Msg: nil, Err: recvErr})
			resultCh <- RunningThreadResult{Prepared: prep, Reply: nil, Err: recvErr}
			return
		}
		if msg == nil {
			continue
		}
		state.appendContentSoFar(msg.Content)
		// Chunk delivery is best-effort; the authoritative content lives in
		// RunningThreadState.contentSoFarBuf.
		trySendChunk(ctx, chunkCh, RunningThreadChunk{Msg: msg, Err: nil})
	}

	replyMsg := &types.ThreadMessage{
		Role:    types.LlmRoleAssistant,
		Content: state.ContentSoFar(),
	}
	if err := thrGrp.finalizeChatOnce(prep, replyMsg); err != nil {
		resultCh <- RunningThreadResult{Prepared: prep, Reply: nil, Err: err}
		return
	}

	resultCh <- RunningThreadResult{Prepared: prep, Reply: replyMsg, Err: nil}
}

func closeStreamOnCancel(ctx context.Context, stream *schema.StreamReader[*types.ThreadMessage]) {
	if ctx == nil || stream == nil {
		return
	}
	<-ctx.Done()
	stream.Close()
}

func trySendChunk(ctx context.Context, ch chan<- RunningThreadChunk, ev RunningThreadChunk) {
	// Best-effort send. If the channel is full, drop it. Callers can utilize
	// ContentSoFar() to get the full history of prior RunningThreadChunk
	select {
	case <-ctx.Done():
		return
	case ch <- ev:
	default:
		// full
	}
}

type threadKey struct{}

// WithThread returns a context with a Thread attached.
func WithThread(ctx context.Context, thread *thread) context.Context {
	return context.WithValue(ctx, threadKey{}, thread)
}

// GetThread retrieves a Thread from a context, if any.
func GetThread(ctx context.Context) (*thread, bool) {
	if v := ctx.Value(threadKey{}); v != nil {
		if t, ok := v.(*thread); ok && t != nil {
			return t, true
		}
	}
	return nil, false
}
