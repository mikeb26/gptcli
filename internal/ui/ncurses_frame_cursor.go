/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import gc "github.com/gbin/goncurses"

// Cursor-related helpers for Frame.

// clampCursorX constrains a logical cursor column to the drawable text
// area for this frame, always reserving the last content column for the
// scrollbar.
func (f *Frame) clampCursorX(x, contentWidth int) int {
	if x < 0 {
		x = 0
	}
	maxCol := contentWidth - 1
	if maxCol > 0 {
		maxCol-- // reserve last column for scrollbar
	}
	if maxCol < 0 {
		maxCol = 0
	}
	if x > maxCol {
		x = maxCol
	}
	return x
}

// drawSoftCursor inverts the cell at (y, x) within the frame window
// using the provided rune as the underlying character.
func (f *Frame) drawSoftCursor(y, x int, ch rune) {
	if y < 0 || x < 0 {
		return
	}
	_ = f.Win.AttrOn(gc.A_REVERSE)
	f.Win.MovePrint(y, x, string(ch))
	_ = f.Win.AttrOff(gc.A_REVERSE)
}

// MoveCursorLeft moves the cursor one position to the left, possibly
// wrapping to the previous line.
func (f *Frame) MoveCursorLeft() {
	if !f.HasCursor {
		return
	}
	if f.cursorCol > 0 {
		f.cursorCol--
		return
	}
	if f.cursorLine > 0 {
		f.cursorLine--
		if f.cursorLine < len(f.lines) {
			f.cursorCol = len(f.lines[f.cursorLine])
		}
	}
}

// MoveCursorRight moves the cursor one position to the right, possibly
// wrapping to the next line.
func (f *Frame) MoveCursorRight() {
	if !f.HasCursor {
		return
	}
	if f.cursorLine < 0 || f.cursorLine >= len(f.lines) {
		return
	}
	line := f.lines[f.cursorLine]
	if f.cursorCol < len(line) {
		f.cursorCol++
		return
	}
	if f.cursorLine < len(f.lines)-1 {
		f.cursorLine++
		f.cursorCol = 0
	}
}

// MoveCursorUp moves the cursor one line up, keeping the closest
// horizontal column.
func (f *Frame) MoveCursorUp() {
	if !f.HasCursor {
		return
	}
	if f.cursorLine == 0 {
		return
	}
	f.cursorLine--
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) && f.cursorCol > len(f.lines[f.cursorLine]) {
		f.cursorCol = len(f.lines[f.cursorLine])
	}
}

// MoveCursorDown moves the cursor one line down, keeping the closest
// horizontal column.
func (f *Frame) MoveCursorDown() {
	if !f.HasCursor {
		return
	}
	if f.cursorLine >= len(f.lines)-1 {
		return
	}
	f.cursorLine++
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) && f.cursorCol > len(f.lines[f.cursorLine]) {
		f.cursorCol = len(f.lines[f.cursorLine])
	}
}
