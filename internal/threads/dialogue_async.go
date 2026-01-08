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

// RunningThreadResult is sent once, when the request has completed (success or
// failure).
type RunningThreadResult struct {
	Reply *types.ThreadMessage
	Err   error
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

	Progress         <-chan types.ProgressEvent
	ApprovalRequests <-chan AsyncApprovalRequest
	AsyncApprover    *AsyncApprover

	Result <-chan RunningThreadResult
	Done   <-chan struct{}

	Cancel context.CancelFunc

	mu sync.RWMutex
	// contentSoFarBuf accumulates the streamed content so far so that background
	// runs can continue even when no UI is actively consuming progress events.
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
// the legacy synchronous chat APIs.
//
// It returns immediately with a RunningThreadState that exposes channels for:
//   - progress events
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
	resultCh := make(chan RunningThreadResult, 1)
	doneCh := make(chan struct{})

	state := &RunningThreadState{
		Prompt:           prompt,
		InvocationID:     invocationID,
		Progress:         progressCh,
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
		state, resultCh, doneCh,
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
	resultCh chan<- RunningThreadResult,
	doneCh chan<- struct{},
) {
	defer close(doneCh)
	defer close(resultCh)
	defer thread.llmClient.UnsubscribeProgress(progressCh, invocationID)

	// Worker-owned flow: prepare the request, stream the assistant reply,
	// finalize/persist, then send the terminal result.

	// Build the user request.
	reqMsg := &types.ThreadMessage{
		Role:    types.LlmRoleUser,
		Content: prompt,
	}

	// Attach a thread reference to the context so lower layers (e.g. tool
	// approvals) can mark the thread blocked/running.
	ctx = WithThread(ctx, thread)

	// Copy persisted dialogue so we don't mutate in-memory state while streaming.
	thread.mu.RLock()
	fullDialogue := make([]*types.ThreadMessage, len(thread.persisted.Dialogue))
	copy(fullDialogue, thread.persisted.Dialogue)
	thread.mu.RUnlock()
	fullDialogue = append(fullDialogue, reqMsg)
	workingDialogue := fullDialogue

	if summarizePrior && len(fullDialogue) > 2 {
		prior := fullDialogue[:len(fullDialogue)-1]
		summaryDialogue, sumErr := summarizeDialogue(ctx, thread.llmClient, prior)
		if sumErr != nil {
			resultCh <- RunningThreadResult{Reply: nil, Err: sumErr}
			return
		}
		workingDialogue = append(summaryDialogue, reqMsg)
	}

	// Ensure an invocation ID exists on the request context.
	ctx, _ = llmclient.EnsureInvocationID(ctx)
	res, err := thread.llmClient.StreamChatCompletion(ctx, workingDialogue)
	if err != nil {
		resultCh <- RunningThreadResult{Reply: nil, Err: err}
		return
	}
	if res == nil || res.Stream == nil {
		err := errors.New("nil stream result")
		resultCh <- RunningThreadResult{Reply: nil, Err: err}
		return
	}
	stream := res.Stream
	defer stream.Close()
	go closeStreamOnCancel(ctx, stream)

	for {
		msg, recvErr := stream.Recv()
		if recvErr != nil {
			if errors.Is(recvErr, io.EOF) {
				break
			}
			resultCh <- RunningThreadResult{Reply: nil, Err: recvErr}
			return
		}
		if msg == nil {
			continue
		}
		state.appendContentSoFar(msg.Content)
	}

	replyMsg := &types.ThreadMessage{
		Role:    types.LlmRoleAssistant,
		Content: state.ContentSoFar(),
	}
	finalDialogue := append(fullDialogue, replyMsg)
	if err := finalizeChatOnce(thrGrp, thread, finalDialogue); err != nil {
		resultCh <- RunningThreadResult{Reply: nil, Err: err}
		return
	}

	resultCh <- RunningThreadResult{Reply: replyMsg, Err: nil}
}

func closeStreamOnCancel(ctx context.Context, stream *schema.StreamReader[*types.ThreadMessage]) {
	if ctx == nil || stream == nil {
		return
	}
	<-ctx.Done()
	stream.Close()
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
