/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

// Input-related helpers for Frame.

// ResetInput clears the internal input buffer and resets cursor/scroll
// state. It is a no-op for frames without HasInput.
func (f *Frame) ResetInput() {
	if !f.HasInput {
		return
	}
	f.lines = [][]rune{{}}
	f.cursorLine = 0
	f.cursorCol = 0
	f.scroll = 0
}

// InsertRune inserts r at the current cursor position within the
// internal input buffer. It is safe to call only when HasInput is true.
func (f *Frame) InsertRune(r rune) {
	if !f.HasInput {
		return
	}
	if f.cursorLine < 0 || f.cursorLine >= len(f.lines) {
		f.cursorLine = len(f.lines) - 1
		if f.cursorLine < 0 {
			f.cursorLine = 0
		}
	}
	line := f.lines[f.cursorLine]
	if f.cursorCol < 0 {
		f.cursorCol = 0
	}
	if f.cursorCol > len(line) {
		f.cursorCol = len(line)
	}
	line = append(line[:f.cursorCol], append([]rune{r}, line[f.cursorCol:]...)...)
	f.lines[f.cursorLine] = line
	f.cursorCol++
}

// InsertNewline splits the current line at the cursor into two lines.
func (f *Frame) InsertNewline() {
	if !f.HasInput {
		return
	}
	if f.cursorLine < 0 || f.cursorLine >= len(f.lines) {
		f.cursorLine = len(f.lines) - 1
		if f.cursorLine < 0 {
			f.cursorLine = 0
		}
	}
	line := f.lines[f.cursorLine]
	if f.cursorCol < 0 {
		f.cursorCol = 0
	}
	if f.cursorCol > len(line) {
		f.cursorCol = len(line)
	}
	before := append([]rune{}, line[:f.cursorCol]...)
	after := append([]rune{}, line[f.cursorCol:]...)

	newLines := make([][]rune, 0, len(f.lines)+1)
	newLines = append(newLines, f.lines[:f.cursorLine]...)
	newLines = append(newLines, before)
	newLines = append(newLines, after)
	newLines = append(newLines, f.lines[f.cursorLine+1:]...)
	f.lines = newLines
	f.cursorLine++
	f.cursorCol = 0
}

// Backspace removes the rune before the cursor, joining lines as needed.
func (f *Frame) Backspace() {
	if !f.HasInput {
		return
	}
	if f.cursorLine == 0 && f.cursorCol == 0 {
		return
	}
	line := f.lines[f.cursorLine]
	if f.cursorCol > 0 {
		if f.cursorCol > len(line) {
			f.cursorCol = len(line)
		}
		line = append(line[:f.cursorCol-1], line[f.cursorCol:]...)
		f.lines[f.cursorLine] = line
		f.cursorCol--
		return
	}
	// At column 0, join with previous line.
	prevLine := f.lines[f.cursorLine-1]
	newCol := len(prevLine)
	joined := append(append([]rune{}, prevLine...), line...)
	newLines := make([][]rune, 0, len(f.lines)-1)
	newLines = append(newLines, f.lines[:f.cursorLine-1]...)
	newLines = append(newLines, joined)
	newLines = append(newLines, f.lines[f.cursorLine+1:]...)
	f.lines = newLines
	f.cursorLine--
	f.cursorCol = newCol
}

// InputString flattens the internal multi-line input buffer into a
// single string using "\n" as the line separator.
func (f *Frame) InputString() string {
	if !f.HasInput {
		return ""
	}
	// We avoid importing strings here to keep dependencies minimal; this
	// helper is mainly for convenience and can be adapted later.
	total := 0
	for _, l := range f.lines {
		total += len(l) + 1 // +1 for potential newline
	}
	if total == 0 {
		return ""
	}
	b := make([]rune, 0, total)
	for i, l := range f.lines {
		b = append(b, l...)
		if i != len(f.lines)-1 {
			b = append(b, '\n')
		}
	}
	return string(b)
}

// visibleContentHeight returns the number of rows available for text
// inside the frame's content area (excluding any border). It is used by
// scrolling helpers to compute page sizes.
func (f *Frame) visibleContentHeight() int {
	_, _, h, _ := f.contentBounds()
	if h < 1 {
		return 1
	}
	return h
}

// EnsureCursorVisible adjusts the scroll offset so that the current
// cursorLine is visible within the frame's content area. It also clamps
// cursorLine, cursorCol, and scroll to valid ranges based on the
// current buffer contents.
func (f *Frame) EnsureCursorVisible() {
	if len(f.lines) == 0 {
		f.cursorLine = 0
		f.cursorCol = 0
		f.scroll = 0
		return
	}

	visible := f.visibleContentHeight()
	if visible < 1 {
		visible = 1
	}
	total := len(f.lines)
	if f.cursorLine < 0 {
		f.cursorLine = 0
	}
	if f.cursorLine > total-1 {
		f.cursorLine = total - 1
	}
	maxScroll := total - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if f.scroll < 0 {
		f.scroll = 0
	}
	if f.scroll > maxScroll {
		f.scroll = maxScroll
	}

	if f.cursorLine < f.scroll {
		f.scroll = f.cursorLine
	} else if f.cursorLine >= f.scroll+visible {
		f.scroll = f.cursorLine - visible + 1
	}
	if f.scroll < 0 {
		f.scroll = 0
	}
	if f.scroll > maxScroll {
		f.scroll = maxScroll
	}

	// Clamp horizontal position to the current line length.
	line := f.lines[f.cursorLine]
	if f.cursorCol < 0 {
		f.cursorCol = 0
	}
	if f.cursorCol > len(line) {
		f.cursorCol = len(line)
	}
}

// ScrollPageUp scrolls the viewport and moves the cursor up by one
// visible page of content.
func (f *Frame) ScrollPageUp() {
	if len(f.lines) == 0 {
		f.cursorLine = 0
		f.cursorCol = 0
		f.scroll = 0
		return
	}
	visible := f.visibleContentHeight()
	if visible < 1 {
		visible = 1
	}
	f.cursorLine -= visible
	if f.cursorLine < 0 {
		f.cursorLine = 0
	}
	f.scroll -= visible
	if f.scroll < 0 {
		f.scroll = 0
	}
	if f.cursorLine < f.scroll {
		f.scroll = f.cursorLine
	}
	// Clamp horizontal column to the new line length.
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) {
		line := f.lines[f.cursorLine]
		if f.cursorCol > len(line) {
			f.cursorCol = len(line)
		}
	}
}

// ScrollPageDown scrolls the viewport and moves the cursor down by one
// visible page of content.
func (f *Frame) ScrollPageDown() {
	if len(f.lines) == 0 {
		f.cursorLine = 0
		f.cursorCol = 0
		f.scroll = 0
		return
	}
	visible := f.visibleContentHeight()
	if visible < 1 {
		visible = 1
	}
	f.cursorLine += visible
	lastIdx := len(f.lines) - 1
	if lastIdx < 0 {
		lastIdx = 0
	}
	if f.cursorLine > lastIdx {
		f.cursorLine = lastIdx
	}
	maxScroll := len(f.lines) - visible
	if maxScroll < 0 {
		maxScroll = 0
	}
	f.scroll += visible
	if f.scroll > maxScroll {
		f.scroll = maxScroll
	}
	if f.cursorLine >= f.scroll+visible {
		f.scroll = f.cursorLine - visible + 1
	}
	// Clamp horizontal column to the new line length.
	if f.cursorLine >= 0 && f.cursorLine < len(f.lines) {
		line := f.lines[f.cursorLine]
		if f.cursorCol > len(line) {
			f.cursorCol = len(line)
		}
	}
}

// MoveHome moves the cursor to the very beginning of the buffer (first
// character of the first line) and scrolls the viewport so that this is
// visible.
func (f *Frame) MoveHome() {
	if len(f.lines) == 0 {
		f.cursorLine = 0
		f.cursorCol = 0
		f.scroll = 0
		return
	}
	f.cursorLine = 0
	f.cursorCol = 0
	f.scroll = 0
}

// MoveEnd moves the cursor to the very end of the buffer (last
// character of the last line) and scrolls the viewport so that this is
// visible.
func (f *Frame) MoveEnd() {
	if len(f.lines) == 0 {
		f.cursorLine = 0
		f.cursorCol = 0
		f.scroll = 0
		return
	}
	visible := f.visibleContentHeight()
	if visible < 1 {
		visible = 1
	}
	f.cursorLine = len(f.lines) - 1
	line := f.lines[f.cursorLine]
	f.cursorCol = len(line)
	if len(f.lines) > visible {
		f.scroll = len(f.lines) - visible
	} else {
		f.scroll = 0
	}
}
