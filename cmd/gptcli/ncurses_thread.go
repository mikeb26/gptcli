/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

const (
	// Additional color pairs for the thread view. These are initialized
	// alongside the menu colors in initUI so they can be reused by any
	// ncurses-based views.
	threadColorUser      int16 = 5
	threadColorAssistant int16 = 6
	threadColorCode      int16 = 7

	// maxAsyncEventsPerTick caps the number of async events we process per UI
	// tick so we don't starve keyboard input when a thread is very chatty
	// (progress updates, streaming chunks, etc.).
	maxAsyncEventsPerTick = 128
)

// threadViewFocus tracks which pane is currently active inside the
// thread view. This determines how keys are interpreted (e.g. whether
// 'q' quits the view or is inserted into the input buffer).
type threadViewFocus int

const (
	focusHistory threadViewFocus = iota
	focusInput
)

// drawNavbar renders a simple status line at the bottom of the
// screen, including mode information and key hints.
func drawNavbar(scr *gc.Window, focus threadViewFocus) {
	maxY, maxX := scr.MaxYX()
	statusY := maxY - 1
	if statusY < 0 {
		return
	}

	segments := []statusSegment{
		{text: "Nav:", bold: false},
		{text: "↑", bold: true},
		{text: "/", bold: false},
		{text: "↓", bold: true},
		{text: "/", bold: false},
		{text: "→", bold: true},
		{text: "/", bold: false},
		{text: "←", bold: true},
		{text: "/", bold: false},
		{text: "PgUp", bold: true},
		{text: "/", bold: false},
		{text: "PgDn", bold: true},
		{text: "/", bold: false},
		{text: "Home", bold: true},
		{text: "/", bold: false},
		{text: "End", bold: true},
		{text: " OtherWin:", bold: false},
		{text: "Tab", bold: true},
		{text: " Send:", bold: false},
		{text: "Ctrl-d", bold: true},
		{text: " Back:", bold: false},
		{text: "ESC", bold: true},
	}
	drawStatusSegments(scr, statusY, maxX, segments, globalUseColors)

}

// drawThreadHeader renders a single-line header for the thread view.
func drawThreadHeader(scr *gc.Window, thread threads.Thread) {
	maxY, maxX := scr.MaxYX()
	if maxY <= 0 {
		return
	}
	header := fmt.Sprintf("Thread: %s", thread.Name())
	if len([]rune(header)) > maxX {
		header = string([]rune(header)[:maxX])
	}

	var attr gc.Char = gc.A_NORMAL
	if globalUseColors {
		attr |= gc.ColorPair(menuColorHeader)
	}
	_ = scr.AttrSet(attr)
	scr.Move(0, 0)
	scr.HLine(0, 0, ' ', maxX)
	scr.MovePrint(0, 0, header)
	_ = scr.AttrSet(gc.A_NORMAL)
}

func applySubmittedPromptToUI(
	scr *gc.Window,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	prompt string,
) (displayBlocks []threads.RenderBlock, historyLines []ui.FrameLine, maxX int) {
	// Immediately reflect the user's input at the end of the history
	// window without mutating the underlying thread yet. We do this by
	// rendering against a temporary thread that includes the pending user
	// message.
	_, maxX = scr.MaxYX()
	displayBlocks = append([]threads.RenderBlock(nil), thread.RenderBlocks()...)
	displayBlocks = append(displayBlocks, threads.RenderBlock{Kind: threads.RenderBlockUserPrompt, Text: prompt})
	historyLines = buildHistoryLines(displayBlocks, maxX)
	historyFrame.SetLines(historyLines)
	historyFrame.MoveEnd()
	historyFrame.Render(false)

	// Clear the input buffer now that we know the async chat has actually
	// started.
	inputFrame.ResetInput()
	inputFrame.Render(true)

	// Show processing status.
	drawThreadInputLabel(scr, "Processing...")
	scr.Refresh()

	return displayBlocks, historyLines, maxX
}

func updateThreadStatusFromProgress(statusText string, toolCalls *int,
	requestCount *int, ev types.ProgressEvent) string {

	var statusPrefix string
	addSuffix := true

	switch ev.Component {
	case types.ProgressComponentModel:
		statusPrefix = "LLM: thinking"
		if ev.Phase == types.ProgressPhaseStart {
			(*requestCount)++
		}
	case types.ProgressComponentTool:
		if ev.Phase == types.ProgressPhaseStart {
			(*toolCalls)++
			statusPrefix = "Tool: running " + ev.DisplayText
			addSuffix = false
		} else {
			statusPrefix = "LLM: thinking"
		}
	default:
		statusPrefix = statusText
	}

	if !addSuffix {
		return statusPrefix
	}

	return fmt.Sprintf("%v (requests:%v toolcalls:%v)...", statusPrefix,
		*requestCount, *toolCalls)
}

func beginAsyncChatFromInputBuffer(
	ctx context.Context,
	scr *gc.Window,
	gptCliCtx *CliContext,
	inputFrame *ui.Frame,
	ncui *ui.NcursesUI,
) (prompt string, state *threads.RunningThreadState, ok bool) {
	// Capture the raw multi-line input and trim it in the same way as the
	// non-UI helpers so that what we display matches what is actually sent
	// to the LLM and eventually persisted in the thread dialogue.
	rawInput := inputFrame.InputString()
	prompt = strings.TrimSpace(rawInput)
	if prompt == "" {
		return "", nil, false
	}

	if gptCliCtx.curThreadGroup == gptCliCtx.archiveThreadGroup {
		_, _ = showErrorRetryModal(ncui, ErrCannotEditArchivedThread.Error())
		return "", nil, false
	}

	state, err := gptCliCtx.curThreadGroup.ChatOnceAsync(
		ctx,
		gptCliCtx.ictx,
		prompt,
		gptCliCtx.curSummaryToggle,
	)
	if err != nil {
		_, _ = showErrorRetryModal(ncui, err.Error())
		return "", nil, false
	}

	// We intentionally do not clear the input buffer or mutate the history
	// view until we know that the async chat has actually started (i.e. the
	// Start event returns successfully). That is handled in
	// processAsyncChatState.
	drawThreadInputLabel(scr, "Processing...")
	scr.Refresh()

	return prompt, state, true
}

// processAsyncChatState drains any currently-available async events
// without blocking the UI.
func processAsyncChatState(
	scr *gc.Window,
	gptCliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	ncui *ui.NcursesUI,
	state *threads.RunningThreadState,
	uiState *asyncChatUIState,
) (done bool, needRedraw bool) {
	if state == nil || uiState == nil {
		return true, false
	}
	// Always prefer the most recent RunningThreadState pointer on reattach.
	uiState.runState = state

	for i := 0; i < maxAsyncEventsPerTick; i++ {
		startCh := uiState.runState.Start
		progressCh := uiState.runState.Progress
		resultCh := uiState.runState.Result
		approvalCh := uiState.runState.ApprovalRequests
		if uiState.startClosed {
			startCh = nil
		}
		if uiState.progressClosed {
			progressCh = nil
		}
		if uiState.resultClosed {
			resultCh = nil
		}
		if uiState.approvalClosed {
			approvalCh = nil
		}

		select {
		case startEv, ok := <-startCh:
			if !ok {
				uiState.startClosed = true
				continue
			}
			// Start is a single-shot channel; treat any receive as terminal.
			uiState.startClosed = true
			if startEv.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, startEv.Err.Error())
				_, maxX := scr.MaxYX()
				persistedLines := buildHistoryLinesForThread(thread, maxX)
				historyFrame.SetLines(persistedLines)
				historyFrame.MoveEnd()
				needRedraw = true
				delete(gptCliCtx.asyncChatUIStates, thread.Id())
				return true, true
			}
			needRedraw = true
		case req, ok := <-approvalCh:
			if !ok {
				uiState.approvalClosed = true
				continue
			}
			state.AsyncApprover.ServeRequest(req)
			historyFrame.Render(false)
			inputFrame.Render(true)
			needRedraw = true
		case ev, ok := <-progressCh:
			if !ok {
				uiState.progressClosed = true
				continue
			}
			uiState.statusText = updateThreadStatusFromProgress(uiState.statusText, &uiState.toolCalls, &uiState.requestCount, ev)
			needRedraw = true
		case res, ok := <-resultCh:
			if !ok {
				uiState.resultClosed = true
				continue
			}
			uiState.gotResult = true
			uiState.resultClosed = true
			if res.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, res.Err.Error())
			}

			// Whether success or error, the thread is now persisted (or failed),
			// so rebuild from the thread's current dialogue.
			_, maxX := scr.MaxYX()
			persistedLines := buildHistoryLinesForThread(thread, maxX)
			historyFrame.SetLines(persistedLines)
			historyFrame.MoveEnd()
			needRedraw = true
			delete(gptCliCtx.asyncChatUIStates, thread.Id())
			return true, true
		default:
			return false, needRedraw
		}
	}

	return false, needRedraw
}

type asyncChatUIState struct {
	displayBlocks []threads.RenderBlock
	historyLines  []ui.FrameLine
	maxX          int
	promptApplied bool

	statusText   string
	toolCalls    int
	requestCount int

	runState *threads.RunningThreadState

	startClosed    bool
	progressClosed bool
	resultClosed   bool
	approvalClosed bool

	gotResult bool

	lastContentLen int
}

func newAsyncChatUIStateAndRender(
	scr *gc.Window,
	gptCliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	state *threads.RunningThreadState,
) *asyncChatUIState {
	if state == nil {
		return nil
	}

	_, maxX := scr.MaxYX()
	displayBlocks := append([]threads.RenderBlock(nil), thread.RenderBlocks()...)
	displayBlocks = append(displayBlocks, threads.RenderBlock{Kind: threads.RenderBlockUserPrompt, Text: state.Prompt})
	historyLines := buildHistoryLines(displayBlocks, maxX)

	uiState := newAsyncChatUIState(gptCliCtx, thread, state, displayBlocks, historyLines, maxX)
	if uiState == nil {
		return nil
	}

	// Render immediately with whatever content is available.
	historyFrame.SetLines(uiState.historyLines)
	historyFrame.MoveEnd()
	rebuildHistory(scr, historyFrame, uiState.displayBlocks, uiState.maxX, state.ContentSoFar())
	inputFrame.Render(true)

	return uiState
}

func newAsyncChatUIState(
	gptCliCtx *CliContext,
	thread threads.Thread,
	state *threads.RunningThreadState,
	displayBlocks []threads.RenderBlock,
	historyLines []ui.FrameLine,
	maxX int,
) *asyncChatUIState {
	if state == nil {
		return nil
	}

	tid := thread.Id()
	if existing, ok := gptCliCtx.asyncChatUIStates[tid]; ok && existing != nil {
		// If we already have a UI state for this thread, prefer to reuse it;
		// it may be holding channels or counters.
		//
		// Refresh the presentation state (render blocks + wrapped history) so the
		// view can detach/reattach and resize cleanly.
		existing.displayBlocks = displayBlocks
		existing.historyLines = historyLines
		existing.maxX = maxX
		existing.promptApplied = true
		existing.runState = state
		if existing.statusText == "" {
			existing.statusText = "LLM: thinking"
		}
		return existing
	}

	uiState := &asyncChatUIState{
		displayBlocks:  displayBlocks,
		historyLines:   historyLines,
		maxX:           maxX,
		promptApplied:  true,
		statusText:     "LLM: thinking",
		runState:       state,
		startClosed:    false,
		progressClosed: false,
		resultClosed:   false,
		approvalClosed: false,
		gotResult:      false,
		lastContentLen: 0,
	}
	gptCliCtx.asyncChatUIStates[tid] = uiState
	return uiState
}

// rebuildHistory reconstructs the history frame lines while a streaming
// response is in flight. It keeps existing history intact and appends a
// temporary assistant message rendered with the same wrapping logic used
// elsewhere.
func rebuildHistory(
	scr *gc.Window,
	historyFrame *ui.Frame,
	baseBlocks []threads.RenderBlock,
	maxX int,
	extraText string,
) {
	blocks := append([]threads.RenderBlock(nil), baseBlocks...)
	if extraText != "" {
		extraBlocks := threads.RenderBlocksFromDialogue([]*types.ThreadMessage{{
			Role:    types.LlmRoleAssistant,
			Content: extraText,
		}})
		blocks = append(blocks, extraBlocks...)
	}

	allLines := buildHistoryLines(blocks, maxX)
	historyFrame.SetLines(allLines)
	historyFrame.MoveEnd()
	historyFrame.Render(false)
	scr.Refresh()
}

func threadViewFocusFromFocusedFrame(focusedFrame, historyFrame, inputFrame *ui.Frame) threadViewFocus {
	if focusedFrame == historyFrame {
		return focusHistory
	}
	return focusInput
}

type threadViewFrames struct {
	historyFrame *ui.Frame
	inputFrame   *ui.Frame

	historyLines []ui.FrameLine
}

func createThreadViewFrames(scr *gc.Window, thread threads.Thread) (*threadViewFrames, error) {
	maxY, maxX := scr.MaxYX()
	frames := &threadViewFrames{}

	frames.historyLines = buildHistoryLinesForThread(thread, maxX)
	// History frame occupies the region between the header and the input
	// label. It is read-only but uses the Frame's cursor/scroll helpers
	// for navigation.
	historyStartY := menuHeaderHeight
	historyEndY := maxY - menuStatusHeight - threadInputHeight
	if historyEndY <= historyStartY {
		historyEndY = historyStartY + 1
	}
	historyH := historyEndY - historyStartY
	if historyH < 1 {
		historyH = 1
	}
	historyW := maxX

	historyFrame, err := ui.NewFrame(scr, historyH, historyW, historyStartY, 0, false, true, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingHistoryFrame, err)
	}
	frames.historyFrame = historyFrame
	frames.historyFrame.SetLines(frames.historyLines)
	// Start with cursor at end of history.
	frames.historyFrame.MoveEnd()

	// Create a Frame to manage the editable multi-line input buffer and
	// its cursor/scroll state. The frame's content area starts on the
	// first row below the input label and extends down to the status bar.
	inputHeight := threadInputHeight
	inputStartY := maxY - menuStatusHeight - inputHeight
	if inputStartY < menuHeaderHeight {
		inputStartY = menuHeaderHeight
	}
	// The label occupies one row; actual editable content lives below it.
	frameY := inputStartY + 1
	frameH := inputHeight - 1
	if frameH < 1 {
		frameH = 1
	}
	frameW := maxX

	inputFrame, err := ui.NewFrame(scr, frameH, frameW, frameY, 0, false, true, true)
	if err != nil {
		frames.historyFrame.Close()
		frames.historyFrame = nil
		return nil, fmt.Errorf("%w: %w", ErrCreatingInputFrame, err)
	}
	frames.inputFrame = inputFrame
	frames.inputFrame.ResetInput()

	return frames, nil
}

func closeThreadViewFrames(frames *threadViewFrames) {
	if frames == nil {
		return
	}
	if frames.historyFrame != nil {
		frames.historyFrame.Close()
		frames.historyFrame = nil
	}
	if frames.inputFrame != nil {
		frames.inputFrame.Close()
		frames.inputFrame = nil
	}
}

func attachToRunningThreadAndUpdateUIState(
	scr *gc.Window,
	gptCliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	ncui *ui.NcursesUI,
) (needRedraw bool) {
	// If this thread has an in-flight run, attach and update the view from the
	// RunningThreadState's buffered content. This allows the user to detach
	// (ESC) and later reattach via the menu.
	if state := thread.GetRunState(); state != nil {
		uiState := newAsyncChatUIStateAndRender(scr, gptCliCtx, thread, historyFrame, inputFrame, state)
		if uiState != nil {
			content := state.ContentSoFar()
			if len(content) != uiState.lastContentLen {
				rebuildHistory(scr, historyFrame, uiState.displayBlocks, uiState.maxX, content)
				uiState.lastContentLen = len(content)
				needRedraw = true
			}
			_, stepRedraw := processAsyncChatState(scr, gptCliCtx, thread, historyFrame, inputFrame, ncui, state, uiState)
			if stepRedraw {
				needRedraw = true
			}
		}
		return needRedraw
	}

	// If the run completed while detached, remove stale UI state.
	delete(gptCliCtx.asyncChatUIStates, thread.Id())
	return false
}

func redrawThreadView(
	scr *gc.Window,
	thread threads.Thread,
	gptCliCtx *CliContext,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	focusedFrame *ui.Frame,
) {
	// First redraw everything that lives directly on the root
	// screen (stdscr). We intentionally refresh this parent
	// window *before* rendering the input frame's sub-window so
	// that the frame's contents are not overwritten by a later
	// scr.Refresh() call.
	scr.Erase()
	drawThreadHeader(scr, thread)
	statusText := ""
	if thread.GetRunState() != nil {
		if uiState, ok := gptCliCtx.asyncChatUIStates[thread.Id()]; ok && uiState != nil {
			statusText = uiState.statusText
		}
		if statusText == "" {
			statusText = "Processing..."
		}
	}
	drawThreadInputLabel(scr, statusText)
	drawNavbar(scr, threadViewFocusFromFocusedFrame(focusedFrame, historyFrame, inputFrame))
	scr.Refresh()

	// Render history and input frames after the root screen so
	// their contents are not overwritten.
	historyFrame.Render(focusedFrame == historyFrame)
	inputFrame.Render(focusedFrame == inputFrame)
}

func processThreadViewKey(
	ctx context.Context,
	scr *gc.Window,
	gptCliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	focusedFrame **ui.Frame,
	ncui *ui.NcursesUI,
	ch gc.Key,
) (exit bool, needRedraw bool) {
	if focusedFrame == nil {
		return false, false
	}
	if *focusedFrame == nil {
		*focusedFrame = inputFrame
	}

	if ch == gc.KEY_TAB {
		if *focusedFrame == inputFrame {
			*focusedFrame = historyFrame
		} else {
			*focusedFrame = inputFrame
		}
		return false, true
	}

	isHistory := *focusedFrame == historyFrame
	isInput := *focusedFrame == inputFrame

	// Exit keys.
	if ch == gc.Key(27) { // ESC
		return true, false
	}
	if isHistory {
		if ch == 'q' || ch == 'Q' || ch == 'd'-'a'+1 { // q/Q, ctrl-d
			return true, false
		}
	}

	// Navigation keys (shared by both history and input frames).
	switch ch {
	case gc.KEY_LEFT:
		(*focusedFrame).MoveCursorLeft()
		return false, true
	case gc.KEY_RIGHT:
		(*focusedFrame).MoveCursorRight()
		return false, true
	case gc.KEY_UP:
		(*focusedFrame).MoveCursorUp()
		(*focusedFrame).EnsureCursorVisible()
		return false, true
	case gc.KEY_DOWN:
		(*focusedFrame).MoveCursorDown()
		(*focusedFrame).EnsureCursorVisible()
		return false, true
	case gc.KEY_PAGEUP:
		(*focusedFrame).ScrollPageUp()
		if isHistory {
			(*focusedFrame).EnsureCursorVisible()
		}
		return false, true
	case gc.KEY_PAGEDOWN:
		(*focusedFrame).ScrollPageDown()
		if isHistory {
			(*focusedFrame).EnsureCursorVisible()
		}
		return false, true
	case gc.KEY_HOME:
		(*focusedFrame).MoveHome()
		return false, true
	case gc.KEY_END:
		(*focusedFrame).MoveEnd()
		return false, true
	}

	if !isInput {
		return false, false
	}

	// Input-only keys.
	switch ch {
	case gc.KEY_BACKSPACE, 127, 8:
		inputFrame.Backspace()
		inputFrame.EnsureCursorVisible()
		return false, true
	case gc.KEY_ENTER, gc.KEY_RETURN:
		inputFrame.InsertNewline()
		inputFrame.EnsureCursorVisible()
		return false, true
	case 'd' - 'a' + 1: // Ctrl-D sends the input buffer
		prompt, state, ok := beginAsyncChatFromInputBuffer(ctx, scr, gptCliCtx, inputFrame, ncui)
		if ok {
			// Immediately reflect the user's submitted prompt in the history
			// pane and clear the input buffer. This restores the pre-async
			// behavior where Ctrl-D visually "sends" the buffer right away.
			displayBlocks, submittedHistoryLines, submittedMaxX := applySubmittedPromptToUI(scr, thread, historyFrame, inputFrame, prompt)
			_ = newAsyncChatUIState(gptCliCtx, thread, state, displayBlocks, submittedHistoryLines, submittedMaxX)
			// Do not block waiting for completion; the UI loop will
			// continue processing async events and the user can detach.
		}
		return false, true
	default:
		// Treat any printable byte (including high‑bit bytes from
		// UTF‑8 sequences) as input. When running in a UTF-8
		// locale, ncurses/GetChar returns each byte of the sequence
		// separately; group those bytes into a single rune so that
		// characters like emoji render correctly.
		if ch >= 32 && ch < 256 {
			r := ui.ReadUTF8KeyRune(scr, ch)
			inputFrame.InsertRune(r)
			inputFrame.EnsureCursorVisible()
			return false, true
		}
	}

	return false, false
}

// runThreadView provides an ncurses-based view for interacting with a
// single thread. It renders the existing dialogue and allows the user
// to enter a multi-line prompt in a 3-line input box. Ctrl-D sends the
// current input buffer via ChatOnce. History and input
// areas are independently scrollable via focus switching (Tab) and
// standard navigation keys. Pressing 'q' or ESC in the history focus
// returns to the menu.
func runThreadView(ctx context.Context, scr *gc.Window,
	gptCliCtx *CliContext, thread threads.Thread) error {
	// Use the terminal cursor for caret display in the thread view.
	_ = gc.Cursor(1)
	defer gc.Cursor(0)

	// Listen for SIGWINCH so we can adjust layout on resize while inside
	// the thread view. This mirrors the behavior of showMenu but keeps
	// all ncurses calls confined to this goroutine.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	_, maxX := scr.MaxYX()
	ncui := gptCliCtx.realUI
	frames, err := createThreadViewFrames(scr, thread)
	if err != nil {
		return err
	}
	defer closeThreadViewFrames(frames)
	historyFrame := frames.historyFrame
	inputFrame := frames.inputFrame
	historyLines := frames.historyLines

	focusedFrame := inputFrame
	needRedraw := true

	for {
		if attachNeedRedraw := attachToRunningThreadAndUpdateUIState(scr, gptCliCtx, thread, historyFrame, inputFrame, ncui); attachNeedRedraw {
			needRedraw = true
		}

		if needRedraw {
			redrawThreadView(scr, thread, gptCliCtx, historyFrame, inputFrame, focusedFrame)
			needRedraw = false
		}

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(scr)
			_, maxX = scr.MaxYX()
			if state := thread.GetRunState(); state != nil {
				uiState := newAsyncChatUIStateAndRender(scr, gptCliCtx, thread, historyFrame, inputFrame, state)
				if uiState != nil {
					uiState.maxX = maxX
					uiState.historyLines = buildHistoryLines(uiState.displayBlocks, maxX)
					historyFrame.SetLines(uiState.historyLines)
					rebuildHistory(scr, historyFrame, uiState.displayBlocks, maxX, state.ContentSoFar())
					uiState.lastContentLen = len(state.ContentSoFar())
				}
			} else {
				historyLines = buildHistoryLinesForThread(thread, maxX)
				historyFrame.SetLines(historyLines)
			}
			needRedraw = true
			continue
		default:
			ch = scr.GetChar()
			if ch == 0 {
				continue
			}
		}

		if ch == gc.KEY_RESIZE {
			resizeScreen(scr)
			_, maxX = scr.MaxYX()
			historyLines = buildHistoryLinesForThread(thread, maxX)
			historyFrame.SetLines(historyLines)
			needRedraw = true
			continue
		}

		exit, keyRedraw := processThreadViewKey(ctx, scr, gptCliCtx, thread, historyFrame, inputFrame, &focusedFrame, ncui, ch)
		if exit {
			return nil
		}
		if keyRedraw {
			needRedraw = true
		}
	}
}
