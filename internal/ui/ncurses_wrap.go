/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package ui

// WrapRunesWithContinuation splits a rune slice into display segments that
// fit within the given width. When width > 1 and the content must be split
// across multiple segments, each non-final segment is sized to (width-1)
// runes so that callers may append a trailing '\\' continuation marker in
// the final column.
//
// The returned segments contain only content runes (no continuation marker).
// The wrapped slice indicates whether the corresponding segment should be
// rendered with a continuation marker.
func WrapRunesWithContinuation(content []rune, width int) (segments [][]rune, wrapped []bool) {
	if width < 1 {
		width = 1
	}

	// Always return at least one segment so that empty logical lines still
	// occupy one display row.
	if len(content) == 0 {
		return [][]rune{{}}, []bool{false}
	}

	for start := 0; start < len(content); {
		remaining := len(content) - start
		// Last segment: it can consume up to width runes.
		if remaining <= width {
			end := start + remaining
			segments = append(segments, content[start:end])
			wrapped = append(wrapped, false)
			break
		}

		// Continuation segment: reserve a cell for the marker when possible.
		segLen := width
		useMarker := false
		if width > 1 {
			segLen = width - 1
			useMarker = true
		}
		if segLen < 1 {
			segLen = 1
		}
		end := start + segLen
		if end > len(content) {
			end = len(content)
			useMarker = false
		}
		segments = append(segments, content[start:end])
		wrapped = append(wrapped, useMarker)
		start = end
	}

	return segments, wrapped
}
