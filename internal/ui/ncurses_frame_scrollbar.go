/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

// Scrollbar-related helpers for Frame.

// renderScrollbar computes and draws the vertical scrollbar in the last
// column of the frame's content area. It follows the same behavior as
// the thread view scrollbars: one-row thumb, optional arrow glyphs, and
// only drawn when total > height.
func (f *Frame) renderScrollbar(y, x, height int, total, offset int) {
	if height <= 0 || total <= height {
		return
	}

	sb := ComputeScrollbar(total, height, offset)
	if !sb.HasScrollbar() {
		return
	}

	for row := 0; row < height; row++ {
		DrawScrollbarCell(f.Win, y+row, row, height, x, sb)
	}
}
