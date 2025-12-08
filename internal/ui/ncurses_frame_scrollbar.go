/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import gc "github.com/gbin/goncurses"

const (
	scrollPointChar  rune = '█'
	scrollTrackChar  rune = '│'
	scrollTopChar    rune = '▲'
	scrollBottomChar rune = '▼'
)

// scrollbar describes the visual layout of a vertical scrollbar for a
// given logical content height and scroll offset.
type scrollbar struct {
	hasScrollbar bool
	useArrows    bool
	barStart     int
	barEnd       int
}

// computeScrollbar calculates how a scrollbar should be rendered for a
// region with the given visible height, total number of logical rows,
// and current scroll offset. The thumb is always one row tall and, when
// there is enough vertical space, arrow glyphs are assumed to occupy the
// first and last rows of the track.
func computeScrollbar(total, height, offset int) scrollbar {
	if height <= 0 || total <= height {
		return scrollbar{hasScrollbar: false}
	}

	sb := scrollbar{hasScrollbar: true}
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

// Scrollbar-related helpers for Frame.

// renderScrollbar computes and draws the vertical scrollbar in the last
// column of the frame's content area. It follows the same behavior as
// the thread view scrollbars: one-row thumb, optional arrow glyphs, and
// only drawn when total > height.
func (f *Frame) renderScrollbar(y, x, height int, total, offset int) {
	if height <= 0 || total <= height {
		return
	}

	sb := computeScrollbar(total, height, offset)
	if !sb.hasScrollbar {
		return
	}

	_ = f.Win.AttrSet(gc.A_NORMAL)

	for row := 0; row < height; row++ {
		var ch rune
		if sb.useArrows {
			if row == 0 {
				ch = scrollTopChar
			} else if row == height-1 {
				ch = scrollBottomChar
			} else {
				ch = scrollTrackChar
				if row >= sb.barStart && row < sb.barEnd {
					ch = scrollPointChar
				}
			}
		} else {
			ch = scrollTrackChar
			if row >= sb.barStart && row < sb.barEnd {
				ch = scrollPointChar
			}
		}
		f.Win.MovePrint(y+row, x, string(ch))
	}
}
