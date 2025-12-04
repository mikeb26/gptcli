/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	gc "github.com/gbin/goncurses"
)

// clampCursorX constrains a logical cursor column to the drawable text
// area for a window that may reserve the last column for a scrollbar.
//
// When reserveLast is true and maxX > 1, the rightmost drawable
// position becomes maxX-2 instead of maxX-1 so that callers can keep
// the cursor from overwriting scrollbar glyphs.
func clampCursorX(x, maxX int, reserveLast bool) int {
	if x < 0 {
		x = 0
	}
	if maxX <= 0 {
		return 0
	}

	maxCol := maxX - 1
	if reserveLast && maxCol > 0 {
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

// drawSoftCursor overlays a simple software cursor by inverting the
// cell at (y, x) using a reversed rendition of ch. The underlying text
// should already have been rendered; this helper only affects
// attributes for the single cell so it can be used on top of colored or
// otherwise formatted content.
func drawSoftCursor(scr *gc.Window, y, x int, ch rune) {
	if y < 0 || x < 0 {
		return
	}
	_ = scr.AttrOn(gc.A_REVERSE)
	scr.MovePrint(y, x, string(ch))
	_ = scr.AttrOff(gc.A_REVERSE)
}
