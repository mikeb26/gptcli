/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

// Cursor-related helpers for Frame.

// clampCursorX constrains a logical cursor column to the drawable text
// area for this frame, always reserving the last content column for the
// scrollbar.
func (f *Frame) clampCursorX(x, contentWidth int) int {
	if x < 0 {
		x = 0
	}
	if contentWidth <= 0 {
		return 0
	}

	// Reserve the last column for the scrollbar.
	maxCol := contentWidth - 1
	if maxCol > 0 {
		maxCol--
	}
	if maxCol < 0 {
		maxCol = 0
	}
	if x > maxCol {
		x = maxCol
	}
	return x
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
			f.cursorCol = len(f.lines[f.cursorLine].Runes)
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
	line := f.lines[f.cursorLine].Runes
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
	if f.HasInput {
		textWidth := f.contentTextWidth()
		pos := f.cursorDisplayPos(textWidth)
		if pos.displayLineIdx <= 0 {
			return
		}
		line, col := f.displayIndexToCursor(textWidth, pos.displayLineIdx-1, pos.x)
		f.cursorLine = line
		f.cursorCol = col
		return
	}
	if f.cursorLine == 0 {
		return
	}
	f.cursorLine--
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) && f.cursorCol > len(f.lines[f.cursorLine].Runes) {
		f.cursorCol = len(f.lines[f.cursorLine].Runes)
	}
}

// MoveCursorDown moves the cursor one line down, keeping the closest
// horizontal column.
func (f *Frame) MoveCursorDown() {
	if !f.HasCursor {
		return
	}
	if f.HasInput {
		textWidth := f.contentTextWidth()
		pos := f.cursorDisplayPos(textWidth)
		total := f.totalDisplayLines(textWidth)
		if total <= 0 {
			return
		}
		if pos.displayLineIdx >= total-1 {
			return
		}
		line, col := f.displayIndexToCursor(textWidth, pos.displayLineIdx+1, pos.x)
		f.cursorLine = line
		f.cursorCol = col
		return
	}
	if f.cursorLine >= len(f.lines)-1 {
		return
	}
	f.cursorLine++
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) && f.cursorCol > len(f.lines[f.cursorLine].Runes) {
		f.cursorCol = len(f.lines[f.cursorLine].Runes)
	}
}
