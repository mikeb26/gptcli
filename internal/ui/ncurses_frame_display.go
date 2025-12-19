/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package ui

// displayPos describes a cursor location in terms of display-wrapped rows.
//
// displayLineIdx is the 0-based index into the rendered (wrapped) rows.
// x is the 0-based column within that wrapped row (excluding the scrollbar
// column). startCol is the 0-based logical rune index in the underlying
// logical line where this wrapped row begins.
type displayPos struct {
	displayLineIdx int
	logicalLineIdx int
	startCol       int
	segLen         int
	x              int
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// contentTextWidth returns the number of columns available for text in the
// frame's content area, always reserving the last content column for the
// scrollbar.
func (f *Frame) contentTextWidth() int {
	_, _, _, contentW := f.contentBounds()
	textW := contentW - 1
	if textW < 1 {
		textW = 1
	}
	return textW
}

// totalDisplayLines returns the number of display rows produced by rendering
// the current logical buffer with soft wrapping.
func (f *Frame) totalDisplayLines(textWidth int) int {
	if len(f.lines) == 0 {
		return 0
	}
	if textWidth < 1 {
		textWidth = 1
	}

	total := 0
	for _, fl := range f.lines {
		segments, _ := WrapRunesWithContinuation(fl.Runes, textWidth)
		total += len(segments)
	}
	return total
}

// cursorDisplayPos computes where the current logical cursor would land in the
// rendered, wrapped view.
func (f *Frame) cursorDisplayPos(textWidth int) displayPos {
	pos := displayPos{displayLineIdx: 0, logicalLineIdx: 0, startCol: 0, segLen: 0, x: 0}
	if len(f.lines) == 0 {
		return pos
	}
	if textWidth < 1 {
		textWidth = 1
	}

	lineIdx := clampInt(f.cursorLine, 0, len(f.lines)-1)
	col := f.cursorCol
	if col < 0 {
		col = 0
	}
	lineRunes := f.lines[lineIdx].Runes
	if col > len(lineRunes) {
		col = len(lineRunes)
	}

	displayIdx := 0
	for li := 0; li < len(f.lines); li++ {
		segments, wrapped := WrapRunesWithContinuation(f.lines[li].Runes, textWidth)
		start := 0
		for si, seg := range segments {
			segEnd := start + len(seg)
			isLastSeg := !wrapped[si]
			if li == lineIdx {
				if col < start {
					// Cursor is before this segment (shouldn't happen); clamp.
					pos.displayLineIdx = displayIdx
					pos.logicalLineIdx = li
					pos.startCol = start
					pos.segLen = len(seg)
					pos.x = 0
					return pos
				}
				if col < segEnd || (isLastSeg && col == segEnd) {
					pos.displayLineIdx = displayIdx
					pos.logicalLineIdx = li
					pos.startCol = start
					pos.segLen = len(seg)
					pos.x = col - start
					return pos
				}
			}
			displayIdx++
			start = segEnd
		}
		if li != lineIdx {
			displayIdx += 0
		}
	}

	// Fallback: end of buffer.
	lastLine := len(f.lines) - 1
	segments, _ := WrapRunesWithContinuation(f.lines[lastLine].Runes, textWidth)
	seg := segments[len(segments)-1]
	pos.displayLineIdx = f.totalDisplayLines(textWidth) - 1
	pos.logicalLineIdx = lastLine
	pos.startCol = len(f.lines[lastLine].Runes) - len(seg)
	pos.segLen = len(seg)
	pos.x = len(seg)
	return pos
}

// displayIndexToCursor maps a desired display row index and x position (column
// within that row) back to a logical cursor position.
func (f *Frame) displayIndexToCursor(textWidth, displayLineIdx, x int) (line, col int) {
	if len(f.lines) == 0 {
		return 0, 0
	}
	if textWidth < 1 {
		textWidth = 1
	}

	total := f.totalDisplayLines(textWidth)
	if total <= 0 {
		return 0, 0
	}
	displayLineIdx = clampInt(displayLineIdx, 0, total-1)
	if x < 0 {
		x = 0
	}

	idx := 0
	for li := 0; li < len(f.lines); li++ {
		segments, _ := WrapRunesWithContinuation(f.lines[li].Runes, textWidth)
		start := 0
		for _, seg := range segments {
			if idx == displayLineIdx {
				segLen := len(seg)
				if x > segLen {
					x = segLen
				}
				return li, start + x
			}
			idx++
			start += len(seg)
		}
	}

	// Fallback: end of buffer.
	lastLine := len(f.lines) - 1
	return lastLine, len(f.lines[lastLine].Runes)
}
