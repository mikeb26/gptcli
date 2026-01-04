/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	gc "github.com/gbin/goncurses"
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
func drawThreadInputLabel(scr *gc.Window, statusText string) {
	maxY, maxX := scr.MaxYX()
	inputHeight := threadInputHeight
	startY := maxY - menuStatusHeight - inputHeight
	if startY < menuHeaderHeight {
		startY = menuHeaderHeight
	}
	if statusText == "" {
		statusText = "What can I help with?"
	}

	if len([]rune(statusText)) > maxX {
		statusText = string([]rune(statusText)[:maxX])
	}
	var sepAttr gc.Char = gc.A_NORMAL
	if globalUseColors {
		sepAttr = gc.ColorPair(menuColorStatus)
	}
	_ = scr.AttrSet(sepAttr)
	scr.Move(startY, 0)
	scr.HLine(startY, 0, ' ', maxX)
	scr.MovePrint(startY, 0, statusText)
	_ = scr.AttrSet(gc.A_NORMAL)
}
