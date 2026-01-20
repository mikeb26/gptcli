/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"github.com/mikeb26/gptcli/internal/ui"
	gc "github.com/rthornton128/goncurses"
)

const (
	// Height (in rows) of the multi-line input box in the thread view.
	// This sits directly above the status bar.
	threadInputHeight = 6
)

// drawThreadInputLabel renders the separator / label row that visually
// separates the history pane from the input area. The editable content
// for the input area itself is now managed by an internal/ui.Frame
// instance owned by runThreadView.
//
// statusText, when non-empty, is appended after the label and can be used
// to display transient thread state (e.g. "Processing...", "LLM: thinking").
func drawThreadInputLabel(cliCtx *CliContext, statusText string) {
	maxY, maxX := cliCtx.rootWin.MaxYX()
	inputHeight := threadInputHeight
	startY := maxY - menuStatusHeight - inputHeight
	if startY < menuHeaderHeight {
		startY = menuHeaderHeight
	}

	if len([]rune(statusText)) > maxX {
		statusText = string([]rune(statusText)[:maxX])
	}
	var sepAttr gc.Char = gc.A_NORMAL
	if cliCtx.toggles.useColors {
		sepAttr = gc.ColorPair(menuColorStatus)
	}
	_ = cliCtx.rootWin.AttrSet(sepAttr)
	// NOTE:
	// - We intentionally avoid mvwhline()/HLine here. Even when embedding
	//   attributes into the chtype, some terminals/curses combos still do not
	//   consistently repaint the full row during incremental refreshes, which
	//   can make the status background look "truncated".
	// - Writing each cell explicitly ensures the full row is touched and uses
	//   the desired background attributes.
	for x := 0; x < maxX; x++ {
		cliCtx.rootWin.MoveAddChar(startY, x, gc.Char(' ')|sepAttr)
	}
	_ = cliCtx.rootWin.TouchLine(startY, 1)
	cliCtx.rootWin.MovePrint(startY, 0, statusText)
	_ = cliCtx.rootWin.AttrSet(gc.A_NORMAL)
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
