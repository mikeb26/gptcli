/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

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

// readUTF8KeyRune attempts to reconstruct a full UTF-8 rune from the
// first key code returned by ncurses. In UTF-8 locales, ncurses/GetChar
// delivers each byte of a multi-byte sequence as a separate "character".
// This helper groups those bytes into a single rune so that characters
// like emoji are stored correctly in the input buffer and rendered as a
// single glyph rather than mojibake.
//
// It is intentionally conservative: if we see anything that looks like a
// special KEY_* value or an invalid UTF-8 continuation, we fall back to
// treating the first byte as an individual rune. This avoids consuming
// unrelated key presses at the cost of occasionally splitting unusual
// input sequences.
func readUTF8KeyRune(scr *gc.Window, first gc.Key) rune {
	// We only ever call this for values in the 0-255 range that ncurses
	// reports for regular character input.
	b0 := byte(int(first) & 0xFF)
	if b0 < 0x80 {
		return rune(b0)
	}

	// Determine expected sequence length from the first byte according to
	// UTF-8 rules.
	var need int
	switch {
	case b0&0xE0 == 0xC0:
		need = 2
	case b0&0xF0 == 0xE0:
		need = 3
	case b0&0xF8 == 0xF0:
		need = 4
	default:
		// Not a valid UTF-8 leading byte; treat as a single-byte rune.
		return rune(b0)
	}

	buf := []byte{b0}
	for len(buf) < need {
		ch := scr.GetChar()
		if ch == 0 {
			// Timeout / no further bytes available; decode whatever we
			// have so far. utf8.DecodeRune will return RuneError on
			// invalid or truncated sequences, which we handle below.
			break
		}
		if ch < 0 || ch > 255 {
			// Likely a KEY_* constant; stop extending this sequence so we
			// don't accidentally consume non-text keys.
			break
		}
		b := byte(int(ch) & 0xFF)
		if b&0xC0 != 0x80 {
			// Not a valid continuation byte; avoid eating what is probably
			// the start of the next key sequence.
			break
		}
		buf = append(buf, b)
	}

	r, _ := utf8.DecodeRune(buf)
	if r == utf8.RuneError {
		// Fall back to the original leading byte so we at least preserve a
		// visible character instead of dropping input entirely.
		return rune(b0)
	}
	return r
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
		lineRunes := visibleLines[i]

		// Leave the last column free for the scrollbar.
		limit := maxX
		if limit > 0 {
			limit--
		}

		// Truncate the line by rune count rather than bytes so we never
		// split a UTF‑8 sequence in the middle. This assumes all runes are
		// single‑cell wide, which is sufficient for common use‑cases.
		textRunes := lineRunes
		if len(textRunes) > limit {
			if limit <= 0 {
				textRunes = nil
			} else if limit == 1 {
				// Just draw a continuation marker.
				textRunes = []rune{'\\'}
			} else {
				// Reserve the last column for a continuation marker.
				textRunes = append(append([]rune{}, textRunes[:limit-1]...), '\\')
			}
		}
		text := string(textRunes)
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

		// Keep the cursor inside the text area, not on top of the
		// scrollbar column. Reserve the last column (maxX-1) for the
		// scrollbar glyph so the cursor never overwrites it.
		cx := clampCursorX(st.cursorCol, maxX, true)

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

		// Use the shared helper so the software cursor logic can be reused
		// by other panes (e.g. the history view) without duplication.
		drawSoftCursor(scr, cy, cx, ch)
	}
}

