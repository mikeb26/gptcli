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
	// (progress updates, etc.).
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
func drawNavbar(cliCtx *CliContext, focus threadViewFocus) {
	maxY, maxX := cliCtx.rootWin.MaxYX()
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
	}
	if cliCtx.curThreadGroup == cliCtx.mainThreadGroup {
		segments = append(segments, []statusSegment{
			{text: " OtherWin:", bold: false},
			{text: "Tab", bold: true},
			{text: " Send:", bold: false},
			{text: "Ctrl-d", bold: true},
		}...)
	}
	segments = append(segments, []statusSegment{
		{text: " Back:", bold: false},
		{text: "ESC", bold: true},
	}...)
	drawStatusSegments(cliCtx.rootWin, statusY, maxX, segments,
		cliCtx.toggles.useColors)

}

// drawThreadHeader renders a single-line header for the thread view.
func drawThreadHeader(cliCtx *CliContext, thread threads.Thread) {
	maxY, maxX := cliCtx.rootWin.MaxYX()
	if maxY <= 0 {
		return
	}
	header := fmt.Sprintf("Thread: %s", thread.Name())
	if len([]rune(header)) > maxX {
		header = string([]rune(header)[:maxX])
	}

	var attr gc.Char = gc.A_NORMAL
	if cliCtx.toggles.useColors {
		attr |= gc.ColorPair(menuColorHeader)
	}
	_ = cliCtx.rootWin.AttrSet(attr)
	cliCtx.rootWin.Move(0, 0)
	cliCtx.rootWin.HLine(0, 0, ' ', maxX)
	cliCtx.rootWin.MovePrint(0, 0, header)
	_ = cliCtx.rootWin.AttrSet(gc.A_NORMAL)
}

func threadViewDisplayBlocks(thread threads.Thread, pendingPrompt string) []threads.RenderBlock {
	blocks := append([]threads.RenderBlock(nil), thread.RenderBlocks()...)
	if pendingPrompt != "" {
		blocks = append(blocks, threads.RenderBlock{Kind: threads.RenderBlockUserPrompt, Text: pendingPrompt})
	}
	return blocks
}

func setHistoryFrameFromBlocks(
	cliCtx *CliContext,
	historyFrame *ui.Frame,
	blocks []threads.RenderBlock,
	extraAssistantText string,
) {
	fullBlocks := append([]threads.RenderBlock(nil), blocks...)
	if extraAssistantText != "" {
		extraBlocks := threads.RenderBlocksFromDialogue([]*types.ThreadMessage{{
			Role:    types.LlmRoleAssistant,
			Content: extraAssistantText,
		}})
		fullBlocks = append(fullBlocks, extraBlocks...)
	}
	_, maxX := cliCtx.rootWin.MaxYX()
	lines := buildHistoryLines(cliCtx, fullBlocks, maxX)
	historyFrame.SetLines(lines)
	historyFrame.MoveEnd()
}

func setHistoryFrameForThread(cliCtx *CliContext, historyFrame *ui.Frame, thread threads.Thread) {
	_, maxX := cliCtx.rootWin.MaxYX()
	historyFrame.SetLines(buildHistoryLinesForThread(cliCtx, thread, maxX))
	historyFrame.MoveEnd()
}

func restoreInputFrameContent(inputFrame *ui.Frame, content string, cursorLine, cursorCol int) {
	if inputFrame == nil {
		return
	}
	inputFrame.ResetInput()
	for _, r := range []rune(content) {
		if r == '\n' {
			inputFrame.InsertNewline()
			continue
		}
		inputFrame.InsertRune(r)
	}

	// Restore cursor position best-effort.
	inputFrame.MoveHome()
	for i := 0; i < cursorLine; i++ {
		inputFrame.MoveCursorDown()
	}
	for i := 0; i < cursorCol; i++ {
		inputFrame.MoveCursorRight()
	}
	inputFrame.EnsureCursorVisible()
}

func beginAsyncChatFromInputBuffer(
	ctx context.Context,
	cliCtx *CliContext,
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

	if cliCtx.curThreadGroup == cliCtx.archiveThreadGroup {
		_, _ = showErrorRetryModal(ncui, ErrCannotEditArchivedThread.Error())
		return "", nil, false
	}

	state, err := cliCtx.curThreadGroup.ChatOnceAsync(
		ctx,
		cliCtx.ictx,
		prompt,
		cliCtx.toggles.summary,
	)
	if err != nil {
		_, _ = showErrorRetryModal(ncui, err.Error())
		return "", nil, false
	}

	// We intentionally do not clear the input buffer or mutate the history view
	// until we know ChatOnceAsync has been successfully started.
	drawThreadInputLabel(cliCtx, "Processing...")
	cliCtx.rootWin.Refresh()

	return prompt, state, true
}

// processAsyncChatState drains any currently-available async events
// without blocking the UI.
func processAsyncChatState(
	cliCtx *CliContext,
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
	uiState.Attach(state)

	for i := 0; i < maxAsyncEventsPerTick; i++ {
		select {
		case req, ok := <-uiState.approvalCh:
			if !ok {
				uiState.approvalCh = nil
				continue
			}
			state.AsyncApprover.ServeRequest(req)
			historyFrame.Render(false)
			inputFrame.Render(true)
			needRedraw = true
		case ev, ok := <-uiState.progressCh:
			if !ok {
				uiState.progressCh = nil
				continue
			}
			uiState.statusText = uiState.statusFromProgress(ev)
			needRedraw = true
		case res, ok := <-uiState.resultCh:
			if !ok {
				uiState.resultCh = nil
				continue
			}
			uiState.resultCh = nil
			if res.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, res.Err.Error())
			}

			// Whether success or error, the thread is now persisted (or failed),
			// so rebuild from the thread's current dialogue.
			setHistoryFrameForThread(cliCtx, historyFrame, thread)
			needRedraw = true
			delete(cliCtx.asyncChatUIStates, thread.Id())
			return true, true
		default:
			return false, needRedraw
		}
	}

	return false, needRedraw
}

type asyncChatUIState struct {
	statusText   string
	toolCalls    int
	requestCount int

	state *threads.RunningThreadState

	progressCh <-chan types.ProgressEvent
	resultCh   <-chan threads.RunningThreadResult
	approvalCh <-chan threads.AsyncApprovalRequest

	lastContentLen int
}

func (s *asyncChatUIState) Attach(state *threads.RunningThreadState) {
	if s == nil || state == nil {
		return
	}
	s.state = state
	// Preserve "closed" state by keeping a channel nil once it has been closed.
	if s.progressCh != nil {
		s.progressCh = state.Progress
	}
	if s.resultCh != nil {
		s.resultCh = state.Result
	}
	if s.approvalCh != nil {
		s.approvalCh = state.ApprovalRequests
	}
}

func (s *asyncChatUIState) statusFromProgress(ev types.ProgressEvent) string {
	statusText := s.statusText

	var statusPrefix string
	addSuffix := true

	switch ev.Component {
	case types.ProgressComponentModel:
		statusPrefix = "LLM: thinking"
		if ev.Phase == types.ProgressPhaseStart {
			s.requestCount++
		}
	case types.ProgressComponentTool:
		if ev.Phase == types.ProgressPhaseStart {
			s.toolCalls++
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

	return fmt.Sprintf("%v (requests:%v toolcalls:%v)...", statusPrefix, s.requestCount, s.toolCalls)
}

func ensureAsyncChatUIState(cliCtx *CliContext, thread threads.Thread, state *threads.RunningThreadState) *asyncChatUIState {
	if cliCtx == nil || state == nil {
		return nil
	}

	tid := thread.Id()
	if existing, ok := cliCtx.asyncChatUIStates[tid]; ok && existing != nil {
		existing.Attach(state)
		if existing.statusText == "" {
			existing.statusText = "LLM: thinking"
		}
		return existing
	}

	uiState := &asyncChatUIState{
		statusText:     "LLM: thinking",
		state:          state,
		progressCh:     state.Progress,
		resultCh:       state.Result,
		approvalCh:     state.ApprovalRequests,
		lastContentLen: -1,
	}
	cliCtx.asyncChatUIStates[tid] = uiState
	return uiState
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
}

func createThreadViewFrames(cliCtx *CliContext, thread threads.Thread) (*threadViewFrames, error) {
	maxY, maxX := cliCtx.rootWin.MaxYX()
	frames := &threadViewFrames{}
	historyLines := buildHistoryLinesForThread(cliCtx, thread, maxX)
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

	historyFrame, err := ui.NewFrame(cliCtx.rootWin, historyH, historyW, historyStartY, 0, false, true, false)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreatingHistoryFrame, err)
	}
	frames.historyFrame = historyFrame
	frames.historyFrame.SetLines(historyLines)
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

	inputFrame, err := ui.NewFrame(cliCtx.rootWin, frameH, frameW, frameY, 0, false, true, true)
	if err != nil {
		frames.historyFrame.Close()
		frames.historyFrame = nil
		return nil, fmt.Errorf("%w: %w", ErrCreatingInputFrame, err)
	}
	frames.inputFrame = inputFrame
	frames.inputFrame.ResetInput()

	return frames, nil
}

func handleThreadViewResize(
	cliCtx *CliContext,
	thread threads.Thread,
	frames **threadViewFrames,
	focusedFrame **ui.Frame,
) (needRedraw bool, err error) {
	oldFrames := *frames
	wasHistoryFocused := false
	if focusedFrame != nil && *focusedFrame == oldFrames.historyFrame {
		wasHistoryFocused = true
	}

	inputContent := ""
	inputLine, inputCol := 0, 0
	if oldFrames.inputFrame != nil {
		inputContent = oldFrames.inputFrame.InputString()
		inputLine, inputCol = oldFrames.inputFrame.Cursor()
	}

	resizeScreen(cliCtx.rootWin)
	closeThreadViewFrames(oldFrames)

	newFrames, err := createThreadViewFrames(cliCtx, thread)
	if err != nil {
		return false, err
	}
	*frames = newFrames
	if focusedFrame != nil {
		if wasHistoryFocused {
			*focusedFrame = newFrames.historyFrame
		} else {
			*focusedFrame = newFrames.inputFrame
		}
	}

	restoreInputFrameContent(newFrames.inputFrame, inputContent, inputLine, inputCol)

	if cliCtx != nil {
		if uiState, ok := cliCtx.asyncChatUIStates[thread.Id()]; ok && uiState != nil && uiState.state != nil {
			state := uiState.state
			blocks := threadViewDisplayBlocks(thread, state.Prompt)
			content := state.ContentSoFar()
			setHistoryFrameFromBlocks(cliCtx, newFrames.historyFrame, blocks, content)
			uiState.lastContentLen = len(content)
			return true, nil
		}
	}

	setHistoryFrameForThread(cliCtx, newFrames.historyFrame, thread)
	if cliCtx != nil {
		delete(cliCtx.asyncChatUIStates, thread.Id())
	}

	return true, nil
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
	cliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	ncui *ui.NcursesUI,
) (needRedraw bool) {
	// If this thread has an in-flight run, attach and update the view from the
	// RunningThreadState's buffered content. This allows the user to detach
	// (ESC) and later reattach via the menu.
	if cliCtx == nil {
		return false
	}
	if uiState, ok := cliCtx.asyncChatUIStates[thread.Id()]; ok && uiState != nil && uiState.state != nil {
		state := uiState.state
		if uiState != nil {
			content := state.ContentSoFar()
			if len(content) != uiState.lastContentLen {
				blocks := threadViewDisplayBlocks(thread, state.Prompt)
				setHistoryFrameFromBlocks(cliCtx, historyFrame, blocks, content)
				uiState.lastContentLen = len(content)
				needRedraw = true
			}
			_, stepRedraw := processAsyncChatState(cliCtx, thread, historyFrame, inputFrame, ncui, state, uiState)
			if stepRedraw {
				needRedraw = true
			}
		}
		return needRedraw
	}

	// If the run completed while detached, remove stale UI state.
	delete(cliCtx.asyncChatUIStates, thread.Id())
	return false
}

func redrawThreadView(
	cliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	focusedFrame *ui.Frame,
) {
	// First redraw everything that lives directly on the root
	// screen (stdscr). We intentionally refresh this parent
	// window *before* rendering the input frame's sub-window so
	// that the frame's contents are not overwritten by a later
	// scr.Refresh() call.
	cliCtx.rootWin.Erase()
	drawThreadHeader(cliCtx, thread)
	statusText := "What can I help with?"
	if cliCtx.curThreadGroup == cliCtx.archiveThreadGroup {
		statusText = "This thread is archived."
	}
	if uiState, ok := cliCtx.asyncChatUIStates[thread.Id()]; ok && uiState != nil && uiState.state != nil {
		statusText = uiState.statusText
		if statusText == "" {
			statusText = "Processing..."
		}
	}
	drawThreadInputLabel(cliCtx, statusText)
	drawNavbar(cliCtx, threadViewFocusFromFocusedFrame(focusedFrame, historyFrame, inputFrame))
	cliCtx.rootWin.Refresh()

	// Render history and input frames after the root screen so
	// their contents are not overwritten.
	historyFrame.Render(focusedFrame == historyFrame)
	inputFrame.Render(focusedFrame == inputFrame)
}

func processThreadViewKey(
	ctx context.Context,
	cliCtx *CliContext,
	thread threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	focusedFrame **ui.Frame,
	ncui *ui.NcursesUI,
	ch gc.Key,
) (exit bool, needRedraw bool) {

	if ch == gc.KEY_TAB {
		if *focusedFrame == inputFrame {
			*focusedFrame = historyFrame
		} else if cliCtx.curThreadGroup == cliCtx.mainThreadGroup {
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
	case 'd' - 'a' + 1: // Ctrl-D sends the input buffer
		if cliCtx.curThreadGroup != cliCtx.mainThreadGroup {
			return false, false
		}
		prompt, state, ok := beginAsyncChatFromInputBuffer(ctx, cliCtx, inputFrame, ncui)
		if ok {
			_ = ensureAsyncChatUIState(cliCtx, thread, state)
			blocks := threadViewDisplayBlocks(thread, prompt)
			setHistoryFrameFromBlocks(cliCtx, historyFrame, blocks, state.ContentSoFar())
			inputFrame.ResetInput()
			inputFrame.EnsureCursorVisible()
			// Do not block waiting for completion; the UI loop will
			// continue processing async events and the user can detach.
		}
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
	default:
		// Treat any printable byte (including high‑bit bytes from
		// UTF‑8 sequences) as input. When running in a UTF-8
		// locale, ncurses/GetChar returns each byte of the sequence
		// separately; group those bytes into a single rune so that
		// characters like emoji render correctly.
		if ch >= 32 && ch < 256 {
			r := ui.ReadUTF8KeyRune(cliCtx.rootWin, ch)
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
// current input buffer via ChatOnceAsync. History and input
// areas are independently scrollable via focus switching (Tab) and
// standard navigation keys. Pressing 'q' or ESC in the history focus
// returns to the menu.
func runThreadView(ctx context.Context, cliCtx *CliContext,
	thread threads.Thread) error {

	// Use the terminal cursor for caret display in the thread view.
	_ = gc.Cursor(1)
	defer gc.Cursor(0)

	// Listen for SIGWINCH so we can adjust layout on resize while inside
	// the thread view. This mirrors the behavior of showMenu but keeps
	// all ncurses calls confined to this goroutine.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	ncui := cliCtx.ui
	frames, err := createThreadViewFrames(cliCtx, thread)
	if err != nil {
		return err
	}
	defer closeThreadViewFrames(frames)
	historyFrame := frames.historyFrame
	inputFrame := frames.inputFrame

	focusedFrame := inputFrame
	if cliCtx.curThreadGroup == cliCtx.archiveThreadGroup {
		focusedFrame = historyFrame
	}
	needRedraw := true

	for {
		if attachNeedRedraw := attachToRunningThreadAndUpdateUIState(cliCtx, thread, historyFrame, inputFrame, ncui); attachNeedRedraw {
			needRedraw = true
		}

		if needRedraw {
			redrawThreadView(cliCtx, thread, historyFrame, inputFrame, focusedFrame)
			needRedraw = false
		}

		var ch gc.Key
		select {
		case <-sigCh:
			if resized, err := handleThreadViewResize(cliCtx, thread, &frames, &focusedFrame); err != nil {
				return err
			} else if resized {
				historyFrame = frames.historyFrame
				inputFrame = frames.inputFrame
				needRedraw = true
			}
			continue
		default:
			ch = cliCtx.rootWin.GetChar()
			if ch == 0 {
				continue
			}
		}

		if ch == gc.KEY_RESIZE {
			if resized, err := handleThreadViewResize(cliCtx, thread, &frames, &focusedFrame); err != nil {
				return err
			} else if resized {
				historyFrame = frames.historyFrame
				inputFrame = frames.inputFrame
				needRedraw = true
			}
			continue
		}

		exit, keyRedraw := processThreadViewKey(ctx, cliCtx, thread, historyFrame, inputFrame, &focusedFrame, ncui, ch)
		if exit {
			return nil
		}
		if keyRedraw {
			needRedraw = true
		}
	}
}
