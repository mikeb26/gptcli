/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"fmt"

	gc "github.com/gbin/goncurses"
)

// Frame represents a logical X-by-Y window region backed by its own
// ncurses subwindow. It provides optional border, cursor, input buffer,
// and vertical scrollbar support so that callers can render scrollable
// text/input areas without reimplementing these behaviors.
//
// A Frame always reserves its last column for a potential vertical
// scrollbar, even when the content currently fits and no scrollbar is
// drawn. This keeps the layout stable as content grows.
//
// The public API is intentionally small and opinionated: callers create
// a Frame with the desired size and behavior flags, then:
//   - call Render with optional external content (e.g. history), and/or
//   - use the input methods (InsertRune, Backspace, etc.) if HasInput is
//     true.
//
// Cursor blinking/timing is left to higher-level code; callers can
// choose whether to show the cursor for a given Render call via the
// blinkOn parameter.
type Frame struct {
	Win *gc.Window

	// Outer dimensions, including any border.
	Height int
	Width  int

	// Behavior flags.
	HasBorder bool
	HasCursor bool
	HasInput  bool

	// Content buffer for editable frames. Non-editable frames can also
	// choose to populate these fields directly and render them via
	// RenderLines.
	lines      [][]rune
	cursorLine int
	cursorCol  int
	scroll     int // first visible logical line index within the content area
}

// NewFrame creates a new Frame backed by a goncurses subwindow. The
// window is created with the given height and width, positioned at
// (startY, startX) relative to the parent window. If hasBorder is true
// the frame draws a simple box border and reduces the inner content
// area accordingly.
//
// The hasCursor flag enables the software cursor overlay; hasInput
// enables the internal multi-line text buffer and editing helpers.
func NewFrame(parent *gc.Window, height, width, startY, startX int, hasBorder, hasCursor, hasInput bool) (*Frame, error) {
	if parent == nil {
		return nil, fmt.Errorf("parent window is nil")
	}

	if height < 1 {
		height = 1
	}
	if width < 1 {
		width = 1
	}

	win, err := gc.NewWindow(height, width, startY, startX)
	if err != nil {
		return nil, err
	}
	_ = win.Keypad(true)

	f := &Frame{
		Win:       win,
		Height:    height,
		Width:     width,
		HasBorder: hasBorder,
		HasCursor: hasCursor,
		HasInput:  hasInput,
	}

	if hasInput {
		f.ResetInput()
	}

	return f, nil
}

// Close deletes the underlying ncurses window.
func (f *Frame) Close() {
	if f == nil || f.Win == nil {
		return
	}
	_ = f.Win.Delete()
}

// contentBounds returns the top-left coordinate and dimensions of the
// inner content area, excluding any border. The height and width values
// are guaranteed to be at least 1.
func (f *Frame) contentBounds() (y, x, h, w int) {
	y, x = 0, 0
	h, w = f.Height, f.Width
	if f.HasBorder {
		y++
		x++
		h -= 2
		w -= 2
	}
	if h < 1 {
		h = 1
	}
	if w < 1 {
		w = 1
	}
	return
}

// Render draws the frame border (if any), the provided non-editable
// content lines (if lines is non-nil), and/or the internal input buffer
// (if HasInput is true). It also draws the vertical scrollbar when
// needed and overlays the software cursor when HasCursor is true and
// blinkOn is true.
//
// The non-editable content argument allows the same Frame abstraction
// to be used for history-style panes: callers can maintain their own
// slice of text lines and pass it to Render, while editable frames rely
// on the internal buffer instead.
func (f *Frame) Render(lines [][]rune, blinkOn bool) {
	if f == nil || f.Win == nil {
		return
	}

	f.Win.Erase()

	if f.HasBorder {
		_ = f.Win.Box(0, 0)
	}

	contentY, contentX, contentH, contentW := f.contentBounds()
	if contentH <= 0 || contentW <= 0 {
		f.Win.Refresh()
		return
	}

	// Choose data source: external lines take precedence when provided,
	// otherwise we fall back to the internal input buffer for editable
	// frames.
	source := lines
	if source == nil && f.HasInput {
		// Convert [][]rune to [][]rune (no-op) for consistent handling.
		source = f.lines
	}

	if source == nil {
		f.Win.Refresh()
		return
	}

	visibleHeight := contentH
	// Always reserve last column of the content area for the scrollbar.
	textWidth := contentW - 1
	if textWidth < 1 {
		textWidth = 1
	}

	// Determine effective scroll offset.
	offset := f.scroll
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}

	// Render visible lines.
	for row := 0; row < visibleHeight; row++ {
		idx := offset + row
		if idx >= len(source) {
			break
		}
		lineRunes := source[idx]

		// Truncate by rune count; when truncated, append a continuation
		// marker in the last text column.
		textRunes := lineRunes
		if len(textRunes) > textWidth {
			if textWidth == 1 {
				textRunes = []rune{'\\'}
			} else {
				textRunes = append(append([]rune{}, textRunes[:textWidth-1]...), '\\')
			}
		}
		f.Win.MovePrint(contentY+row, contentX, string(textRunes))
	}

	// Draw scrollbar in the last column of the content area.
	f.renderScrollbar(contentY, contentX+contentW-1, visibleHeight, len(source), offset)

	// Software cursor overlay.
	if f.HasCursor && blinkOn {
		cy := contentY + (f.cursorLine - offset)
		if cy < contentY {
			cy = contentY
		}
		if cy >= contentY+visibleHeight {
			cy = contentY + visibleHeight - 1
		}

		cx := f.clampCursorX(f.cursorCol, contentW)
		cx += contentX

		// Determine underlying rune for cursor cell.
		ch := ' '
		if f.cursorLine >= 0 && f.cursorLine < len(source) {
			lineRunes := source[f.cursorLine]
			if f.cursorCol >= 0 && f.cursorCol < len(lineRunes) {
				ch = lineRunes[f.cursorCol]
			}
		}

		f.drawSoftCursor(cy, cx, ch)
	}

	f.Win.Refresh()
}
