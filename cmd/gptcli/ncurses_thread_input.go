/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"

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
func drawThreadInputLabel(scr *gc.Window, focus threadViewFocus) {
	maxY, maxX := scr.MaxYX()
	inputHeight := threadInputHeight
	startY := maxY - menuStatusHeight - inputHeight
	if startY < menuHeaderHeight {
		startY = menuHeaderHeight
	}
	label := "Input"
	if focus == focusInput {
		label = "Input*"
	}

	labelText := fmt.Sprintf("[%s]", label)
	if len([]rune(labelText)) > maxX {
		labelText = string([]rune(labelText)[:maxX])
	}
	var sepAttr gc.Char = gc.A_REVERSE
	if globalUseColors {
		sepAttr = gc.ColorPair(menuColorStatus)
	}
	_ = scr.AttrSet(sepAttr | gc.A_BOLD)
	scr.Move(startY, 0)
	scr.HLine(startY, 0, ' ', maxX)
	scr.MovePrint(startY, 0, labelText)
	_ = scr.AttrSet(gc.A_NORMAL)
}


