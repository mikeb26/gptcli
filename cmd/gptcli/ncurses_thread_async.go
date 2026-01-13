/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
)

const asyncStatusProcessing = "Processing"
const asyncStatusThinking = "Thinking"
const asyncStatusToolRun = "Running"
const asyncStatusAnswering = "Answering"
const asyncStatusIdle = "What can I help with?"
const asyncStatusArchived = "This thread is archived."

type threadViewAsyncChatState struct {
	toolCalls    int
	requestCount int

	state *threads.RunningThreadState

	progressCh <-chan types.ProgressEvent
	resultCh   <-chan threads.RunningThreadResult
	approvalCh <-chan threads.AsyncApprovalRequest

	lastContentLen int

	startedAt     time.Time
	stepStartedAt time.Time

	lastStatusUpdate time.Time
	lastStatusPrefix string
}

func (tvUI *threadViewUI) beginAsyncChat(
	ctx context.Context,
) (prompt string, ok bool) {
	// Capture the raw multi-line input and trim it in the same way as the
	// non-UI helpers so that what we display matches what is actually sent
	// to the LLM and eventually persisted in the thread dialogue.
	rawInput := tvUI.inputFrame.InputString()
	prompt = strings.TrimSpace(rawInput)
	if prompt == "" {
		return "", false
	}

	if tvUI.isArchived {
		_, _ = showErrorRetryModal(tvUI.cliCtx.ui,
			ErrCannotEditArchivedThread.Error())
		return "", false
	}

	state, err := tvUI.thread.ChatOnceAsync(ctx,
		tvUI.cliCtx.ictx, prompt, tvUI.cliCtx.toggles.summary)
	if err != nil {
		_, _ = showErrorRetryModal(tvUI.cliCtx.ui, err.Error())
		return "", false
	}

	tvUI.setRunningState(state)

	// We intentionally do not clear the input buffer or mutate the history view
	// until we know ChatOnceAsync has been successfully started.
	drawThreadInputLabel(tvUI.cliCtx, tvUI.statusText)
	tvUI.cliCtx.rootWin.Refresh()

	return prompt, true
}

func (tvUI *threadViewUI) setRunningState(state *threads.RunningThreadState) {
	now := time.Now()
	tvUI.statusText = asyncStatusProcessing
	tvUI.running.lastStatusPrefix = asyncStatusProcessing
	tvUI.running.toolCalls = 0
	tvUI.running.requestCount = 0
	tvUI.running.state = state
	tvUI.running.progressCh = state.Progress
	tvUI.running.resultCh = state.Result
	tvUI.running.approvalCh = state.ApprovalRequests
	tvUI.running.lastContentLen = -1
	tvUI.running.startedAt = now
	tvUI.running.stepStartedAt = now
	tvUI.running.lastStatusUpdate = now
}

func (tvUI *threadViewUI) clearRunningState() {
	tvUI.statusText = asyncStatusIdle
	if tvUI.cliCtx.curThreadGroup == tvUI.cliCtx.archiveThreadGroup {
		tvUI.statusText = asyncStatusArchived
	}
	tvUI.running = threadViewAsyncChatState{}
}

func (running *threadViewAsyncChatState) formatStatus(now time.Time) string {
	if running == nil {
		return ""
	}

	prefix := running.lastStatusPrefix

	stepSec := int(now.Sub(running.stepStartedAt).Seconds())
	totalSec := int(now.Sub(running.startedAt).Seconds())

	return fmt.Sprintf("%v(%vs of %vs)... [requests:%v toolcalls:%v]",
		prefix, stepSec, totalSec, running.requestCount, running.toolCalls)
}

func (s *threadViewAsyncChatState) statusFromProgress(ev types.ProgressEvent) string {
	now := time.Now()
	s.stepStartedAt = now
	s.lastStatusUpdate = now

	switch ev.Component {
	case types.ProgressComponentModel:
		if ev.Phase == types.ProgressPhaseStart {
			s.requestCount++
			s.lastStatusPrefix = asyncStatusThinking
		} else {
			if s.state != nil && s.state.ContentSoFar() == "" {
				s.lastStatusPrefix = asyncStatusProcessing
			} else {
				s.lastStatusPrefix = asyncStatusAnswering
			}
		}
	case types.ProgressComponentTool:
		if ev.Phase == types.ProgressPhaseStart {
			s.toolCalls++
			s.lastStatusPrefix = asyncStatusToolRun + ev.DisplayText
		} else {
			s.lastStatusPrefix = asyncStatusProcessing
		}
	}

	return s.formatStatus(now)
}

func (tvUI *threadViewUI) tickStatus() {
	if tvUI.running.state == nil {
		return
	}

	now := time.Now()
	if now.Sub(tvUI.running.lastStatusUpdate) < 200*time.Millisecond {
		return
	}

	tvUI.running.lastStatusUpdate = now
	tvUI.statusText = tvUI.running.formatStatus(now)
	drawThreadInputLabel(tvUI.cliCtx, tvUI.statusText)
}

func (tvUI *threadViewUI) processAsyncChat() (needRedraw bool) {
	if tvUI.running.state == nil {
		return false
	}
	state := tvUI.running.state
	content := state.ContentSoFar()
	if len(content) != tvUI.running.lastContentLen {
		blocks := threadViewDisplayBlocks(tvUI.thread, state.Prompt)
		tvUI.setHistoryFrameFromBlocks(blocks, content)
		tvUI.running.lastContentLen = len(content)
		needRedraw = true
	}
	_, stepRedraw := tvUI.processAsyncChatEvents()
	if stepRedraw {
		needRedraw = true
	}

	// Keep status durations ticking even if no new progress events arrive.
	tvUI.tickStatus()
	return needRedraw
}

// processAsyncChatEvents drains any currently-available async events
// without blocking the UI.
func (tvUI *threadViewUI) processAsyncChatEvents() (done bool, needRedraw bool) {
	// maxAsyncEventsPerTick caps the number of async events we process per UI
	// tick so we don't starve keyboard input when a thread is very chatty
	// (progress updates, etc.).
	const maxAsyncEventsPerTick = 128

	if tvUI.running.state == nil {
		return true, false
	}
	state := tvUI.running.state

	for i := 0; i < maxAsyncEventsPerTick; i++ {
		select {
		case req, ok := <-tvUI.running.approvalCh:
			if !ok {
				tvUI.running.approvalCh = nil
				continue
			}
			state.AsyncApprover.ServeRequest(req)
			tvUI.historyFrame.Render(false)
			tvUI.inputFrame.Render(true)
			needRedraw = true
		case ev, ok := <-tvUI.running.progressCh:
			if !ok {
				tvUI.running.progressCh = nil
				continue
			}
			tvUI.statusText = tvUI.running.statusFromProgress(ev)
			drawThreadInputLabel(tvUI.cliCtx, tvUI.statusText)
		case res, ok := <-tvUI.running.resultCh:
			if !ok {
				tvUI.running.resultCh = nil
				continue
			}
			tvUI.running.resultCh = nil
			if res.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(tvUI.cliCtx.ui, res.Err.Error())
			}

			// Whether success or error, the thread is now persisted (or failed),
			// so rebuild from the thread's current dialogue.
			tvUI.setHistoryFrameForThread()
			needRedraw = true
			tvUI.clearRunningState()
			return true, true
		default:
			return false, needRedraw
		}
	}

	return false, needRedraw
}
