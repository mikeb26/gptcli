/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import gc "github.com/rthornton128/goncurses"

const (
	scrollPointChar  rune = '█'
	scrollTrackChar  rune = '│'
	scrollTopChar    rune = '▲'
	scrollBottomChar rune = '▼'
)

// Scrollbar describes the visual layout of a vertical scrollbar for a
// given logical content height and scroll offset.
//
// Callers should treat this as an opaque value and use DrawScrollbarCell
// to render individual rows.
type Scrollbar struct {
	hasScrollbar bool
	useArrows    bool
	barStart     int
	barEnd       int
}

func (s Scrollbar) HasScrollbar() bool { return s.hasScrollbar }

// DrawScrollbarColumn computes and draws a full-height vertical scrollbar
// column.
//
// topY is the first row (absolute coordinate in win) for the scrollbar
// track, height is the number of rows in the track, and col is the X
// coordinate for the scrollbar column.
func DrawScrollbarColumn(win *gc.Window, topY, height, col, total, offset int) {
	if win == nil || height <= 0 || col < 0 {
		return
	}

	sb := ComputeScrollbar(total, height, offset)
	if !sb.HasScrollbar() {
		return
	}

	for row := 0; row < height; row++ {
		DrawScrollbarCell(win, topY+row, row, height, col, sb)
	}
}

// ComputeScrollbar calculates how a scrollbar should be rendered for a
// region with the given visible height, total number of logical rows,
// and current scroll offset. The thumb is always one row tall and, when
// there is enough vertical space, arrow glyphs are assumed to occupy the
// first and last rows of the track.
func ComputeScrollbar(total, height, offset int) Scrollbar {
	if height <= 0 || total <= height {
		return Scrollbar{hasScrollbar: false}
	}

	sb := Scrollbar{hasScrollbar: true}
	sb.useArrows = height >= 3
	scrollRange := total - height
	if scrollRange < 1 {
		scrollRange = 1
	}
	barSize := 1

	clamped := offset
	if clamped < 0 {
		clamped = 0
	}
	if clamped > scrollRange {
		clamped = scrollRange
	}

	if sb.useArrows {
		trackHeight := height - 2
		if trackHeight < 1 {
			trackHeight = 1
		}
		trackSteps := trackHeight - barSize
		if trackSteps < 1 {
			trackSteps = 1
		}
		pos := clamped * trackSteps / scrollRange
		sb.barStart = 1 + pos
		sb.barEnd = sb.barStart + barSize
	} else {
		track := height - barSize
		if track < 1 {
			track = 1
		}
		pos := clamped * track / scrollRange
		sb.barStart = pos
		sb.barEnd = sb.barStart + barSize
	}

	return sb
}

// DrawScrollbarCell renders a single cell of a vertical scrollbar in the
// given window. The logical scrollbar geometry (including whether arrows
// are used and where the thumb is) is provided via sb, and rowIdx is the
// zero-based index of the current row within the scrollbar track
// (0..height-1). screenY is the absolute Y coordinate in the window at
// which the cell should be drawn, and col is the X coordinate for the
// scrollbar column.
//
// Callers typically compute sb once via ComputeScrollbar and then invoke
// this helper from inside their row-rendering loops.
func DrawScrollbarCell(scr *gc.Window, screenY, rowIdx, height, col int, sb Scrollbar) {
	if scr == nil || !sb.hasScrollbar || col < 0 {
		return
	}

	// Scrollbars are always drawn with a neutral attribute so they remain
	// visually distinct from any colored content in the row.
	_ = scr.AttrSet(gc.A_NORMAL)

	var ch rune
	if sb.useArrows {
		// Top and bottom rows show arrow glyphs; the rows between form the
		// scroll track that hosts the thumb.
		if rowIdx == 0 {
			ch = scrollTopChar
		} else if rowIdx == height-1 {
			ch = scrollBottomChar
		} else {
			ch = scrollTrackChar
			if rowIdx >= sb.barStart && rowIdx < sb.barEnd {
				ch = scrollPointChar
			}
		}
	} else {
		// No room for arrows; just render track + thumb.
		ch = scrollTrackChar
		if rowIdx >= sb.barStart && rowIdx < sb.barEnd {
			ch = scrollPointChar
		}
	}

	// Use MovePrint with a single-rune string so that UTF-8 scrollbar
	// glyphs (e.g. box-drawing characters and arrows) are rendered
	// correctly via ncurses' multibyte path instead of the single-byte
	// waddch API.
	scr.MovePrint(screenY, col, string(ch))
}
