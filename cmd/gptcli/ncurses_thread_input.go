/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"
	"strings"

	gc "github.com/gbin/goncurses"
)

const (
	// Height (in rows) of the multi-line input box in the thread view.
	// This sits directly above the status bar.
	threadInputHeight = 6
)

// inputState holds the editable multi-line input buffer used in the
// thread view, along with cursor and scroll position.
type inputState struct {
	lines      [][]rune
	cursorLine int
	cursorCol  int
	scroll     int // first visible logical line index in the input area
}

// reset recomputes the internal representation of the input buffer
// for a fresh, empty state.
func (st *inputState) reset() {
	st.lines = [][]rune{{}}
	st.cursorLine = 0
	st.cursorCol = 0
	st.scroll = 0
}

// insertRune inserts r at the current cursor position.
func (st *inputState) insertRune(r rune) {
	line := st.lines[st.cursorLine]
	if st.cursorCol < 0 {
		st.cursorCol = 0
	}
	if st.cursorCol > len(line) {
		st.cursorCol = len(line)
	}
	line = append(line[:st.cursorCol], append([]rune{r}, line[st.cursorCol:]...)...)
	st.lines[st.cursorLine] = line
	st.cursorCol++
}

// insertNewline splits the current line at the cursor into two lines.
func (st *inputState) insertNewline() {
	line := st.lines[st.cursorLine]
	if st.cursorCol < 0 {
		st.cursorCol = 0
	}
	if st.cursorCol > len(line) {
		st.cursorCol = len(line)
	}
	before := append([]rune{}, line[:st.cursorCol]...)
	after := append([]rune{}, line[st.cursorCol:]...)

	newLines := make([][]rune, 0, len(st.lines)+1)
	newLines = append(newLines, st.lines[:st.cursorLine]...)
	newLines = append(newLines, before)
	newLines = append(newLines, after)
	newLines = append(newLines, st.lines[st.cursorLine+1:]...)
	st.lines = newLines
	st.cursorLine++
	st.cursorCol = 0
}

// backspace removes the rune before the cursor, joining lines as needed.
func (st *inputState) backspace() {
	if st.cursorLine == 0 && st.cursorCol == 0 {
		return
	}
	line := st.lines[st.cursorLine]
	if st.cursorCol > 0 {
		if st.cursorCol > len(line) {
			st.cursorCol = len(line)
		}
		line = append(line[:st.cursorCol-1], line[st.cursorCol:]...)
		st.lines[st.cursorLine] = line
		st.cursorCol--
		return
	}
	// At column 0, join with previous line.
	prevLine := st.lines[st.cursorLine-1]
	newCol := len(prevLine)
	joined := append(append([]rune{}, prevLine...), line...)
	newLines := make([][]rune, 0, len(st.lines)-1)
	newLines = append(newLines, st.lines[:st.cursorLine-1]...)
	newLines = append(newLines, joined)
	newLines = append(newLines, st.lines[st.cursorLine+1:]...)
	st.lines = newLines
	st.cursorLine--
	st.cursorCol = newCol
}

// moveCursorLeft moves the cursor one position to the left, possibly
// wrapping to the previous line.
func (st *inputState) moveCursorLeft() {
	if st.cursorCol > 0 {
		st.cursorCol--
		return
	}
	if st.cursorLine > 0 {
		st.cursorLine--
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// moveCursorRight moves the cursor one position to the right, possibly
// wrapping to the next line.
func (st *inputState) moveCursorRight() {
	line := st.lines[st.cursorLine]
	if st.cursorCol < len(line) {
		st.cursorCol++
		return
	}
	if st.cursorLine < len(st.lines)-1 {
		st.cursorLine++
		st.cursorCol = 0
	}
}

// moveCursorUp moves the cursor one line up, keeping the closest
// horizontal column.
func (st *inputState) moveCursorUp() {
	if st.cursorLine == 0 {
		return
	}
	st.cursorLine--
	if st.cursorCol > len(st.lines[st.cursorLine]) {
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// moveCursorDown moves the cursor one line down, keeping the closest
// horizontal column.
func (st *inputState) moveCursorDown() {
	if st.cursorLine >= len(st.lines)-1 {
		return
	}
	st.cursorLine++
	if st.cursorCol > len(st.lines[st.cursorLine]) {
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// toString flattens the multi-line input buffer to a single string.
func (st *inputState) toString() string {
	parts := make([]string, len(st.lines))
	for i, l := range st.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// drawThreadInput renders the multi-line input box above the status bar
// and, when focusInput is active, overlays a simple software "blinking
// cursor" at the logical cursor position. The real terminal cursor is
// kept hidden (see initUI) so this function is responsible for giving
// the user a clear visual caret.
func drawThreadInput(scr *gc.Window, st *inputState, focus threadViewFocus,
	blinkOn bool) {

	maxY, maxX := scr.MaxYX()
	inputHeight := threadInputHeight
	startY := maxY - menuStatusHeight - inputHeight
	if startY < menuHeaderHeight {
		startY = menuHeaderHeight
	}
	endY := startY + inputHeight
	label := "Input"
	if focus == focusInput {
		label = "Input*"
	}

	// Draw a dedicated separator / mode-line style row between the
	// history pane and the input area. This mirrors the visual split
	// that editors like Emacs use between windows: a full-width line
	// with a distinct background carrying the "[Input]" label.
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

	// Clear the input text area below the separator line.
	for y := startY + 1; y < endY; y++ {
		scr.Move(y, 0)
		scr.HLine(y, 0, ' ', maxX)
	}

	// Render logical lines into the remaining rows below the label.
	visibleLines := st.lines
	if st.scroll > 0 && st.scroll < len(visibleLines) {
		visibleLines = visibleLines[st.scroll:]
	}

	// Compute scrollbar layout for the input area using the shared
	// helper. The scrollbar is rendered in the last column of the input
	// region. We only include the text rows below the label in the
	// scrollbar height.
	total := len(st.lines)
	height := inputHeight - 1 // rows available for text under the label
	sbX := maxX - 1
	sb := computeScrollbar(total, height, st.scroll)

	// For simplicity, map each logical line to a single visual row with
	// soft truncation. This keeps input editing predictable while still
	// supporting multi-line prompts.
	for i := 0; i < height && i < len(visibleLines); i++ {
		rowY := startY + 1 + i
		text := string(visibleLines[i])
		runes := []rune(text)
		// Leave the last column free for the scrollbar.
		limit := maxX
		if limit > 0 {
			limit--
		}
		if len(runes) > limit {
			// Indicate wrap with a trailing '\\'.
			if limit > 1 {
				text = string(runes[:limit-1]) + "\\"
			} else if limit == 1 {
				text = "\\"
			} else {
				text = ""
			}
		}
		scr.MovePrint(rowY, 0, text)

		// Draw scrollbar for this row via the shared helper. When the
		// content fits within the visible area the helper becomes a no-op
		// and the last column is left blank.
		if sbX >= 0 {
			// For the input area, rowIdx is relative to the first text row
			// below the label, so it starts at 0 for the first visible
			// logical line.
			rowIdx := i
			if rowIdx < height {
				drawScrollbarCell(scr, rowY, rowIdx, height, sbX, sb)
			}
		}
	}

	// Software blinking cursor for the input area. When the input pane has
	// focus we draw a reversed character at the logical cursor position
	// whenever blinkOn is true. The underlying text is still rendered
	// normally above; this overlay simply inverts the cell so the caret is
	// always visible even with the real terminal cursor hidden.
	if focus == focusInput && blinkOn {
		cy := startY + 1 + (st.cursorLine - st.scroll)
		if cy < startY+1 {
			cy = startY + 1
		}
		if cy >= endY {
			cy = endY - 1
		}

		cx := st.cursorCol
		// Keep the cursor inside the text area, not on top of the
		// scrollbar column. Reserve the last column (maxX-1) for the
		// scrollbar glyph so the cursor never overwrites it.
		if maxX > 1 && cx >= maxX-1 {
			cx = maxX - 2
		} else if maxX == 1 {
			cx = 0
		}
		if cx < 0 {
			cx = 0
		}

		// Determine the underlying rune at the cursor position so we can
		// invert it instead of drawing a generic block. When the cursor is
		// at the end of the line we simply highlight a space.
		ch := ' '
		if st.cursorLine >= 0 && st.cursorLine < len(st.lines) {
			lineRunes := st.lines[st.cursorLine]
			if st.cursorCol >= 0 && st.cursorCol < len(lineRunes) {
				ch = lineRunes[st.cursorCol]
			}
		}

		_ = scr.AttrOn(gc.A_REVERSE)
		// Use MovePrint with a single-rune string so that the software
		// cursor correctly inverts UTF-8 characters without corrupting
		// ncurses attributes.
		scr.MovePrint(cy, cx, string(ch))
		_ = scr.AttrOff(gc.A_REVERSE)
	}
}
