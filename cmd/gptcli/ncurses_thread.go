/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/scm"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
	gc "github.com/rthornton128/goncurses"
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

type threadViewUI struct {
	cliCtx       *CliContext
	thread       threads.Thread
	isArchived   bool
	running      threadViewAsyncChatState
	statusText   string
	inputFrame   *ui.Frame
	historyFrame *ui.Frame
	focusedFrame *ui.Frame
	workDir      string
}

// automatically add AGENTS.md to the system prompt when present in the user's
// repository
func (tvUI *threadViewUI) getSystemPrompt() string {
	if tvUI.workDir == "" {
		return prompts.SystemMsg
	}

	// best effort
	content, err := os.ReadFile(filepath.Join(tvUI.workDir, "AGENTS.md"))
	if err != nil {
		return prompts.SystemMsg
	}

	return fmt.Sprintf("%v\nThe user's repository contains an AGENTS.md with the following additional instructions:\n%v\n:",
		prompts.SystemMsg, string(content))
}

func lookupOrCreateThreadViewUI(ctx context.Context, cliCtx *CliContext,
	thread threads.Thread, isArchivedIn bool) *threadViewUI {

	tid := thread.Id()
	if existing, ok := cliCtx.threadViews[tid]; ok && existing != nil {
		return existing
	}
	tvUI := &threadViewUI{
		cliCtx:     cliCtx,
		thread:     thread,
		isArchived: isArchivedIn,
	}
	tvUI.clearRunningState()
	cliCtx.threadViews[tid] = tvUI
	wd, _ := os.Getwd()
	_, err := cliCtx.scmClient.RepoStatusString(ctx, wd)
	if err == nil {
		tvUI.workDir = wd
	}

	return tvUI
}

func (tvUI *threadViewUI) createThreadViewFrames() error {
	maxY, maxX := tvUI.cliCtx.rootWin.MaxYX()

	historyLines := buildHistoryLinesForThread(tvUI.cliCtx, tvUI.thread, maxX)
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

	var err error
	tvUI.historyFrame, err = ui.NewFrame(tvUI.cliCtx.rootWin, historyH, historyW,
		historyStartY, 0, false, true, false)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCreatingHistoryFrame, err)
	}
	tvUI.historyFrame.SetLines(historyLines)
	// Start with cursor at end of history.
	tvUI.historyFrame.MoveEnd()

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

	tvUI.inputFrame, err = ui.NewFrame(tvUI.cliCtx.rootWin, frameH, frameW, frameY,
		0, false, true, true)
	if err != nil {
		tvUI.historyFrame.Close()
		tvUI.historyFrame = nil
		return fmt.Errorf("%w: %w", ErrCreatingInputFrame, err)
	}
	tvUI.inputFrame.ResetInput()

	return nil
}

func (tvUI *threadViewUI) handleThreadViewResize() (needRedraw bool, err error) {
	oldFocus := tvUI.getFocus()
	inputLine, inputCol := 0, 0
	inputContent := tvUI.inputFrame.InputString()
	inputLine, inputCol = tvUI.inputFrame.Cursor()

	resizeScreen(tvUI.cliCtx.rootWin)

	tvUI.closeThreadViewFrames()

	err = tvUI.createThreadViewFrames()
	if err != nil {
		return false, err
	}
	if oldFocus == focusHistory {
		tvUI.focusedFrame = tvUI.historyFrame
	} else {
		tvUI.focusedFrame = tvUI.inputFrame
	}

	restoreInputFrameContent(tvUI.inputFrame, inputContent, inputLine, inputCol)

	tvUI.syncHistoryFrameWithCurrentThreadState()

	return true, nil
}

func (tvUI *threadViewUI) closeThreadViewFrames() {
	if tvUI.historyFrame != nil {
		tvUI.historyFrame.Close()
		tvUI.historyFrame = nil
	}
	if tvUI.inputFrame != nil {
		tvUI.inputFrame.Close()
		tvUI.inputFrame = nil
	}
	tvUI.focusedFrame = nil
}

func (tvUI *threadViewUI) redrawThreadView(ctx context.Context) {
	// First redraw everything that lives directly on the root
	// screen (stdscr). We intentionally refresh this parent
	// window *before* rendering the input frame's sub-window so
	// that the frame's contents are not overwritten by a later
	// scr.Refresh() call.
	tvUI.cliCtx.rootWin.Erase()
	tvUI.drawThreadHeader(ctx)
	drawThreadInputLabel(tvUI.cliCtx, tvUI.statusText)
	drawNavbar(tvUI.cliCtx, tvUI.getFocus(), tvUI.isArchived)
	tvUI.cliCtx.rootWin.Refresh()

	// Render history and input frames after the root screen so
	// their contents are not overwritten.
	tvUI.historyFrame.Render(tvUI.getFocus() == focusHistory)
	tvUI.inputFrame.Render(tvUI.getFocus() == focusInput)
}

func (tvUI *threadViewUI) processThreadViewKey(
	ctx context.Context,
	ch gc.Key,
) (exit bool, needRedraw bool) {

	if ch == gc.KEY_TAB {
		if tvUI.getFocus() == focusInput {
			tvUI.focusedFrame = tvUI.historyFrame
		} else if !tvUI.isArchived {
			tvUI.focusedFrame = tvUI.inputFrame
		}

		return false, true
	}

	isHistory := tvUI.getFocus() == focusHistory
	// Exit keys.
	if ch == gc.Key(27) { // ESC
		return true, false
	}

	// Navigation keys (shared by both history and input frames).
	switch ch {
	case gc.KEY_LEFT:
		tvUI.focusedFrame.MoveCursorLeft()
		return false, true
	case gc.KEY_RIGHT:
		tvUI.focusedFrame.MoveCursorRight()
		return false, true
	case gc.KEY_UP:
		tvUI.focusedFrame.MoveCursorUp()
		tvUI.focusedFrame.EnsureCursorVisible()
		return false, true
	case gc.KEY_DOWN:
		tvUI.focusedFrame.MoveCursorDown()
		tvUI.focusedFrame.EnsureCursorVisible()
		return false, true
	case gc.KEY_PAGEUP:
		tvUI.focusedFrame.ScrollPageUp()
		if isHistory {
			tvUI.focusedFrame.EnsureCursorVisible()
		}
		return false, true
	case gc.KEY_PAGEDOWN:
		tvUI.focusedFrame.ScrollPageDown()
		if isHistory {
			tvUI.focusedFrame.EnsureCursorVisible()
		}
		return false, true
	case gc.KEY_HOME:
		tvUI.focusedFrame.MoveHome()
		return false, true
	case gc.KEY_END:
		tvUI.focusedFrame.MoveEnd()
		return false, true
	case 'c':
		if isHistory {
			return false, tvUI.launchCommitFromThreadView(ctx)
		} // else do not return; inputFrame needs to process 'c' as input
	case 'd':
		if isHistory {
			return false, tvUI.launchDiffToolFromThreadView(ctx)
		} // else do not return; inputFrame needs to process 'd' as
		// input
	case 'd' - 'a' + 1: // Ctrl-D sends the input buffer
		if tvUI.isArchived {
			return false, false
		}
		prompt, ok := tvUI.beginAsyncChat(ctx)
		if ok {
			state := tvUI.running.state
			blocks := threadViewDisplayBlocks(tvUI.thread, prompt)
			tvUI.setHistoryFrameFromBlocks(blocks, state.ContentSoFar())
			tvUI.inputFrame.ResetInput()
			tvUI.inputFrame.EnsureCursorVisible()
			// Do not block waiting for completion; the UI loop will
			// continue processing async events and the user can detach.
		}
		return false, true
	}

	if isHistory {
		return false, false
	}

	// Input-only keys.
	switch ch {
	case gc.KEY_BACKSPACE, 127, 8:
		tvUI.inputFrame.Backspace()
		tvUI.inputFrame.EnsureCursorVisible()
		return false, true
	case gc.KEY_ENTER, gc.KEY_RETURN:
		tvUI.inputFrame.InsertNewline()
		tvUI.inputFrame.EnsureCursorVisible()
		return false, true
	default:
		// Treat any printable byte (including high‑bit bytes from
		// UTF‑8 sequences) as input. When running in a UTF-8
		// locale, ncurses/GetChar returns each byte of the sequence
		// separately; group those bytes into a single rune so that
		// characters like emoji render correctly.
		if ch >= 32 && ch < 256 {
			r := ui.ReadUTF8KeyRune(tvUI.cliCtx.rootWin, ch)
			tvUI.inputFrame.InsertRune(r)
			tvUI.inputFrame.EnsureCursorVisible()
			return false, true
		}
	}

	return false, false
}

func (tvUI *threadViewUI) launchDiffToolFromThreadView(ctx context.Context) (needRedraw bool) {
	if tvUI.workDir == "" {
		return false
	}

	// Suspend curses so the difftool can use the terminal.
	gc.DefProgMode()
	gc.End()
	err := tvUI.cliCtx.scmClient.DiffTool(ctx, tvUI.workDir, scm.DiffScopeUncommitted)
	gc.ResetProgMode()
	gc.UpdatePanels()
	gc.StdScr().Refresh()
	if err != nil {
		_ = tvUI.cliCtx.ui.Confirm(err.Error())
	}

	return true
}

func (tvUI *threadViewUI) launchCommitFromThreadView(ctx context.Context) (needRedraw bool) {
	if tvUI.workDir == "" {
		return false
	}

	opts := scm.CommitOptions{}

	for {
		// This uses the user's configured git editor (git commit without -m).
		// Suspend curses so the editor can use the terminal.
		gc.DefProgMode()
		gc.End()
		untracked, err := tvUI.cliCtx.scmClient.Commit(ctx, tvUI.workDir, opts)
		gc.ResetProgMode()
		gc.UpdatePanels()
		gc.StdScr().Refresh()

		if err == nil {
			return true
		}

		if !errors.Is(err, scm.ErrUntrackedFiles) {
			_ = tvUI.cliCtx.ui.Confirm(err.Error())
			return true
		}

		// Ask whether to include each untracked file.
		if opts.IncludeUntracked == nil {
			opts.IncludeUntracked = make(map[string]bool)
		}
		for _, f := range untracked.Filename {
			// If already decided (e.g. retry), don't ask again.
			if _, ok := opts.IncludeUntracked[f]; ok {
				continue
			}

			prompt := fmt.Sprintf("Include currently untracked %v in this commit?", f)
			defaultNo := false
			include, selErr := tvUI.cliCtx.ui.SelectBool(
				prompt,
				types.UIOption{Key: "y", Label: "Yes, include"},
				types.UIOption{Key: "n", Label: "No, ignore"},
				&defaultNo,
			)
			if selErr != nil {
				_ = tvUI.cliCtx.ui.Confirm(selErr.Error())
				return true
			}
			opts.IncludeUntracked[f] = include
		}
		// Retry.
	}
}

// runThreadView provides an ncurses-based view for interacting with a
// single thread. It renders the existing dialogue and allows the user
// to enter a multi-line prompt in a 3-line input box. Ctrl-D sends the
// current input buffer via ChatOnceAsync. History and input
// areas are independently scrollable via focus switching (Tab) and
// standard navigation keys. Pressing 'q' or ESC in the history focus
// returns to the menu.
func runThreadView(ctx context.Context, cliCtx *CliContext,
	thread threads.Thread, isArchived bool) error {

	// Use the terminal cursor for caret display in the thread view.
	_ = gc.Cursor(1)
	defer gc.Cursor(0)

	// Listen for SIGWINCH so we can adjust layout on resize while inside
	// the thread view. This mirrors the behavior of showMenu but keeps
	// all ncurses calls confined to this goroutine.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	tvUI := lookupOrCreateThreadViewUI(ctx, cliCtx, thread, isArchived)
	err := tvUI.createThreadViewFrames()
	if err != nil {
		return err
	}
	defer tvUI.closeThreadViewFrames()

	// If we are re-entering a thread that has an in-flight async run, the
	// persisted thread dialogue won't include the pending user prompt yet.
	// Initialize history from the running state so the user sees their prompt
	// immediately even if the model hasn't streamed any new tokens.
	tvUI.syncHistoryFrameWithCurrentThreadState()

	tvUI.focusedFrame = tvUI.inputFrame
	if tvUI.isArchived {
		tvUI.focusedFrame = tvUI.historyFrame
	}
	// Important: draw the thread view at least once before we service any
	// in-flight async approval requests. Otherwise, if the thread is currently
	// blocked awaiting approval and the approval request is already queued, the
	// approval modal can appear over the previous screen (the menu view).
	//
	// We still process async events immediately afterwards; this just ensures the
	// user sees the thread view first.
	tvUI.redrawThreadView(ctx)
	needRedraw := false

	for {
		if runningNeedRedraw := tvUI.processAsyncChat(); runningNeedRedraw {
			needRedraw = true
		}

		if needRedraw {
			tvUI.redrawThreadView(ctx)
			needRedraw = false
		}

		var ch gc.Key
		select {
		case <-sigCh:
			if resized, err := tvUI.handleThreadViewResize(); err != nil {
				return err
			} else if resized {
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
			if resized, err := tvUI.handleThreadViewResize(); err != nil {
				return err
			} else if resized {
				needRedraw = true
			}
			continue
		}

		exit, keyRedraw := tvUI.processThreadViewKey(ctx, ch)
		if exit {
			tvUI.thread.Access()
			return nil
		}
		if keyRedraw {
			needRedraw = true
		}
	}
}
