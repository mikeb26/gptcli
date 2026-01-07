/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

// AdjustListViewport clamps and scrolls a list viewport so that the
// selected row is always visible.
//
// total is the number of list items.
// viewHeight is the number of visible rows available for the list.
// selected is the currently selected item index.
// offset is the current scroll offset (index of the first visible item).
func AdjustListViewport(total, viewHeight int, selected, offset *int) {
	if selected == nil || offset == nil {
		return
	}

	if viewHeight <= 0 || total == 0 {
		*offset = 0
		if total == 0 {
			*selected = 0
		} else if *selected >= total {
			*selected = total - 1
		} else if *selected < 0 {
			*selected = 0
		}
		return
	}

	if *selected < 0 {
		*selected = 0
	}
	if *selected >= total {
		*selected = total - 1
	}

	if *offset > *selected {
		*offset = *selected
	}
	if *selected >= *offset+viewHeight {
		*offset = *selected - viewHeight + 1
	}

	maxOffset := total - viewHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if *offset > maxOffset {
		*offset = maxOffset
	}
	if *offset < 0 {
		*offset = 0
	}
}
