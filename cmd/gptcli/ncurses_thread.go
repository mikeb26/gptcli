/* Copyright © 2025 Mike Brown. All Rights Reserved.
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
func drawThreadHeader(scr *gc.Window, thread *threads.Thread) {
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
	thread *threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	prompt string,
) (displayThread threads.Thread, historyLines []ui.FrameLine, maxX int) {
	// Immediately reflect the user's input at the end of the history
	// window without mutating the underlying thread yet. We do this by
	// rendering against a temporary thread that includes the pending user
	// message.
	_, maxX = scr.MaxYX()
	displayThread = *(thread.Copy())
	userMsg := &types.ThreadMessage{
		Role:    types.LlmRoleUser,
		Content: prompt,
	}
	displayThread.AppendDialogue(userMsg)
	historyLines = buildHistoryLines(&displayThread, maxX)
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

	return displayThread, historyLines, maxX
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
		_, _ = showErrorRetryModal(ncui, "Cannot edit archived thread; use unarchive first")
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

func processAsyncChatState(
	scr *gc.Window,
	thread *threads.Thread,
	historyFrame *ui.Frame,
	inputFrame *ui.Frame,
	ncui *ui.NcursesUI,
	prompt string,
	state *threads.RunningThreadState,
) (promptApplied bool) {
	if state == nil {
		return false
	}
	defer state.Close()

	var displayThread threads.Thread
	var historyLines []ui.FrameLine
	maxX := 0

	statusText := "LLM: thinking"
	startCh := state.Start
	chunkCh := state.Chunk
	resultCh := state.Result
	progressCh := state.Progress
	approvalCh := state.ApprovalRequests
	var buffer strings.Builder
	gotResult := false
	toolCalls := 0
	requestCount := 0

	for startCh != nil || chunkCh != nil || !gotResult {
		drawThreadInputLabel(scr, statusText)
		scr.Refresh()

		select {
		case req := <-approvalCh:
			// Tools and other background workers can request user interaction
			// via the async approver. Serve those requests here so that all
			// ncurses rendering stays confined to this goroutine.
			state.AsyncApprover.ServeRequest(req)
			// Redraw the underlying frames after the modal closes.
			historyFrame.Render(false)
			inputFrame.Render(true)
			scr.Refresh()
		case ev, ok := <-progressCh:
			if !ok {
				progressCh = nil
				continue
			}
			statusText = updateThreadStatusFromProgress(statusText, &toolCalls,
				&requestCount, ev)
		case start, ok := <-startCh:
			if !ok {
				startCh = nil
				continue
			}
			startCh = nil
			if start.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, start.Err.Error())
				return false
			}
			if start.Prepared == nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, "nil prepared chat")
				return false
			}

			// The async chat has successfully started; now reflect the user's
			// prompt in the UI and clear the input buffer.
			displayThread, historyLines, maxX = applySubmittedPromptToUI(scr, thread, historyFrame, inputFrame, prompt)
			promptApplied = true
		case ce, ok := <-chunkCh:
			if !ok {
				// Stream completed.
				chunkCh = nil
				if gotResult {
					return promptApplied
				}
				continue
			}
			if ce.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, ce.Err.Error())
				return promptApplied
			}
			if ce.Msg == nil {
				continue
			}

			// In case we see chunks before the Start event is observed (it
			// should not happen, but avoid a broken UI if it does).
			if !promptApplied {
				displayThread, historyLines, maxX = applySubmittedPromptToUI(scr, thread, historyFrame, inputFrame, prompt)
				promptApplied = true
			}

			// As soon as we start receiving assistant chunks, we're
			// in the "answering" phase.
			statusText = "LLM: answering"
			buffer.WriteString(ce.Msg.Content)
			rebuildHistory(scr, historyFrame, &displayThread, historyLines, maxX, buffer.String())
		case res, ok := <-resultCh:
			if !ok {
				resultCh = nil
				continue
			}
			resultCh = nil
			gotResult = true
			if res.Err != nil {
				state.Stop()
				_, _ = showErrorRetryModal(ncui, res.Err.Error())
				return promptApplied
			}
		}
	}

	return promptApplied
}

// rebuildHistory reconstructs the history frame lines while a streaming
// response is in flight. It keeps existing history intact and appends a
// temporary assistant message rendered with the same wrapping logic used
// elsewhere.
func rebuildHistory(
	scr *gc.Window,
	historyFrame *ui.Frame,
	thread *threads.Thread,
	historyLines []ui.FrameLine,
	maxX int,
	extraText string,
) {
	// Rebuild a fresh slice so that wrapping stays consistent with
	// existing history behavior.
	allLines := make([]ui.FrameLine, len(historyLines))
	copy(allLines, historyLines)

	if extraText != "" {
		// Render the in-flight assistant text as its own block. We reuse
		// buildHistoryLines on a temporary thread to avoid duplicating
		// wrapping logic.
		tmpThread := *thread
		tmpMsg := &types.ThreadMessage{
			Role:    types.LlmRoleAssistant,
			Content: extraText,
		}
		tmpThread.AppendDialogue(tmpMsg)
		extraLines := buildHistoryLines(&tmpThread, maxX)
		// Only keep the lines corresponding to the new assistant message by
		// dropping the original history length.
		if len(extraLines) > len(historyLines) {
			allLines = append(allLines, extraLines[len(historyLines):]...)
		}
	}

	historyFrame.SetLines(allLines)
	historyFrame.MoveEnd()
	historyFrame.Render(false)
	scr.Refresh()
}

// runThreadView provides an ncurses-based view for interacting with a
// single thread. It renders the existing dialogue and allows the user
// to enter a multi-line prompt in a 3-line input box. Ctrl-D sends the
// current input buffer via ChatOnce. History and input
// areas are independently scrollable via focus switching (Tab) and
// standard navigation keys. Pressing 'q' or ESC in the history focus
// returns to the menu.
func runThreadView(ctx context.Context, scr *gc.Window,
	gptCliCtx *CliContext, thread *threads.Thread) error {
	// Listen for SIGWINCH so we can adjust layout on resize while inside
	// the thread view. This mirrors the behavior of showMenu but keeps
	// all ncurses calls confined to this goroutine.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	maxY, maxX := scr.MaxYX()
	ncui := gptCliCtx.realUI
	historyLines := buildHistoryLines(thread, maxX)
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
		return fmt.Errorf("creating history frame: %w", err)
	}
	defer historyFrame.Close()
	historyFrame.SetLines(historyLines)
	// Start with cursor at end of history.
	historyFrame.MoveEnd()

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
		return fmt.Errorf("creating input frame: %w", err)
	}
	defer inputFrame.Close()
	inputFrame.ResetInput()

	focus := focusInput
	needRedraw := true

	// Simple blink state for the software cursor in the input area. We
	// toggle blinkOn after a small number of input polling ticks so it
	// blinks even when the user is idle.
	blinkOn := true
	blinkCounter := 0
	const blinkTicks = 6 // ~300ms at the menu's 50ms timeout

	for {
		if needRedraw {
			// First redraw everything that lives directly on the root
			// screen (stdscr). We intentionally refresh this parent
			// window *before* rendering the input frame's sub-window so
			// that the frame's contents are not overwritten by a later
			// scr.Refresh() call.
			scr.Erase()
			drawThreadHeader(scr, thread)
			drawThreadInputLabel(scr, "")
			drawNavbar(scr, focus)
			scr.Refresh()
			// Render history and input frames after the root screen so
			// their contents are not overwritten.
			historyFrame.Render(blinkOn && focus == focusHistory)
			inputFrame.Render(blinkOn && focus == focusInput)
			needRedraw = false
		}

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(scr)
			maxY, maxX = scr.MaxYX()
			historyLines = buildHistoryLines(thread, maxX)
			historyFrame.SetLines(historyLines)
			needRedraw = true
			continue
		default:
			ch = scr.GetChar()
			if ch == 0 {
				// Timeout/no key pressed: advance the blink timer for the
				// software cursor in the active pane.
				blinkCounter++
				if blinkCounter >= blinkTicks {
					blinkCounter = 0
					blinkOn = !blinkOn
					needRedraw = true
				}
				continue
			}
		}

		switch focus {
		case focusHistory:
			switch ch {
			case 'q', 'Q', 'd' - 'a' + 1, gc.Key(27): // q/Q, ctrl-d, ESC
				return nil
			case gc.KEY_LEFT:
				historyFrame.MoveCursorLeft()
				needRedraw = true
			case gc.KEY_RIGHT:
				historyFrame.MoveCursorRight()
				needRedraw = true
			case gc.KEY_UP:
				historyFrame.MoveCursorUp()
				historyFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_DOWN:
				historyFrame.MoveCursorDown()
				historyFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_PAGEUP:
				historyFrame.ScrollPageUp()
				historyFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_PAGEDOWN:
				historyFrame.ScrollPageDown()
				historyFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_HOME:
				historyFrame.MoveHome()
				needRedraw = true
			case gc.KEY_END:
				historyFrame.MoveEnd()
				needRedraw = true
			case gc.KEY_RESIZE:
				resizeScreen(scr)
				maxY, maxX = scr.MaxYX()
				historyLines = buildHistoryLines(thread, maxX)
				historyFrame.SetLines(historyLines)
				needRedraw = true
			case gc.KEY_TAB:
				focus = focusInput
				needRedraw = true
			}
		case focusInput:
			switch ch {
			case gc.KEY_RESIZE:
				resizeScreen(scr)
				maxY, maxX = scr.MaxYX()
				historyLines = buildHistoryLines(thread, maxX)
				historyFrame.SetLines(historyLines)
				needRedraw = true
			case gc.KEY_TAB:
				focus = focusHistory
				needRedraw = true
			case gc.Key(27): // ESC
				return nil
			case gc.KEY_HOME:
				// Move to the very beginning of the input buffer (first
				// character of the first line), mirroring HOME behavior in
				// the history view.
				inputFrame.MoveHome()
				needRedraw = true
			case gc.KEY_END:
				// Move to the very end of the input buffer (last character
				// of the last line), mirroring END behavior in the history
				// view.
				// Move to the very end of the input buffer (last character
				// of the last line), mirroring END behavior in the history
				// view.
				inputFrame.MoveEnd()
				needRedraw = true
			case gc.KEY_PAGEUP:
				// Scroll and move the cursor up by one visible page.
				inputFrame.ScrollPageUp()
				needRedraw = true
			case gc.KEY_PAGEDOWN:
				// Scroll and move the cursor down by one visible page.
				inputFrame.ScrollPageDown()
				needRedraw = true
			case gc.KEY_LEFT:
				inputFrame.MoveCursorLeft()
				needRedraw = true
			case gc.KEY_RIGHT:
				inputFrame.MoveCursorRight()
				needRedraw = true
			case gc.KEY_UP:
				inputFrame.MoveCursorUp()
				inputFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_DOWN:
				inputFrame.MoveCursorDown()
				inputFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_BACKSPACE, 127, 8:
				inputFrame.Backspace()
				inputFrame.EnsureCursorVisible()
				needRedraw = true
			case gc.KEY_ENTER, gc.KEY_RETURN:
				inputFrame.InsertNewline()
				inputFrame.EnsureCursorVisible()
				needRedraw = true
			case 'd' - 'a' + 1: // Ctrl-D sends the input buffer
				prompt, state, ok := beginAsyncChatFromInputBuffer(ctx, scr, gptCliCtx, inputFrame, ncui)
				if ok {
					_ = processAsyncChatState(scr, thread, historyFrame, inputFrame, ncui, prompt, state)
					// Rebuild the history from the persisted thread now that
					// the async chat is complete.
					maxY, maxX = scr.MaxYX()
					historyLines = buildHistoryLines(thread, maxX)
					historyFrame.SetLines(historyLines)
					historyFrame.MoveEnd()
				}
				needRedraw = true
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
					needRedraw = true
				}
			}
		}
	}
}
