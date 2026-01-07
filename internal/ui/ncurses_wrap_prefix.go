/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"strings"

	gc "github.com/gbin/goncurses"
)

// WrapTextWithPrefix converts a logical block of text into a slice of
// FrameLine values suitable for rendering in a Frame.
//
// The first rendered line is prefixed with prefix. Subsequent logical
// lines (split by '\n') and any wrapped continuation segments are
// indented with spaces matching the rune width of prefix.
//
// width is the total available columns for the rendered line, including
// both the prefix and a possible trailing '\\' wrap marker.
func WrapTextWithPrefix(prefix, text string, width int, attr gc.Char) []FrameLine {
	if width < 1 {
		width = 1
	}
	if attr == 0 {
		attr = gc.A_NORMAL
	}

	prefixRunes := []rune(prefix)
	indentRunes := make([]rune, len(prefixRunes))
	for i := range indentRunes {
		indentRunes[i] = ' '
	}

	parts := strings.Split(text, "\n")
	if len(parts) == 0 {
		parts = []string{""}
	}

	out := make([]FrameLine, 0, len(parts))
	for pi, part := range parts {
		basePrefix := prefixRunes
		if pi > 0 {
			basePrefix = indentRunes
		}

		avail := width - len(basePrefix)
		if avail < 1 {
			avail = 1
		}

		segments, wrappedFlags := WrapRunesWithContinuation([]rune(part), avail)
		for si, seg := range segments {
			segPrefix := basePrefix
			if si > 0 {
				segPrefix = indentRunes
			}

			lineRunes := make([]rune, 0, len(segPrefix)+len(seg)+1)
			lineRunes = append(lineRunes, segPrefix...)
			lineRunes = append(lineRunes, seg...)
			if wrappedFlags[si] {
				lineRunes = append(lineRunes, '\\')
			}

			out = append(out, FrameLine{Runes: lineRunes, Attr: attr})
		}
	}

	return out
}
