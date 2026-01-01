/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"strings"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/ui"
)

// buildHistoryLines converts the logical RenderBlocks for a thread
// into a flat slice of ui.FrameLine values, applying prefixes ("You:",
// "LLM:") and soft wrapping with a trailing '\\' on wrapped
// segments. The resulting slice is suitable for direct line-by-line
// rendering in the history pane via a ui.Frame.
func buildHistoryLines(thread *threads.Thread, width int) []ui.FrameLine {
	// We need at least two columns: one for text and one for the history
	// frame's scrollbar. Below that threshold we simply omit history
	// rendering.
	if width <= 1 {
		return nil
	}
	blocks := thread.RenderBlocks()
	lines := make([]ui.FrameLine, 0)

	// The history frame reserves its last column for a scrollbar, so we
	// only have (width-1) columns available for text. Wrapping must obey
	// this limit or Frame.Render will apply an additional truncation
	// (including its own '\\' marker) and drop characters at wrap
	// boundaries.
	textWidth := width - 1
	if textWidth < 1 {
		textWidth = 1
	}
	for _, b := range blocks {
		var prefix string
		attr := gc.A_NORMAL

		switch b.Kind {
		case threads.RenderBlockUserPrompt:
			prefix = "You: "
			if globalUseColors {
				attr = gc.ColorPair(threadColorUser)
			} else {
				attr = gc.A_BOLD
			}
		case threads.RenderBlockAssistantText:
			prefix = "LLM: "
			if globalUseColors {
				attr = gc.ColorPair(threadColorAssistant)
			} else {
				attr = gc.A_NORMAL
			}
		case threads.RenderBlockAssistantCode:
			prefix = "LLM: "
			if globalUseColors {
				attr = gc.ColorPair(threadColorCode)
			} else {
				attr = gc.A_BOLD
			}
		}

		// Split on logical newlines first.
		parts := strings.Split(b.Text, "\n")
		for i, part := range parts {
			linePrefix := prefix
			if i > 0 {
				// Subsequent lines in the same block are aligned with
				// the content rather than repeating the role label.
				linePrefix = strings.Repeat(" ", len([]rune(prefix)))
			}

			contentRunes := []rune(part)
			prefixRunes := []rune(linePrefix)
			// avail is the total number of text cells available for this
			// line, including a possible '\\' continuation marker.
			avail := textWidth - len(prefixRunes)
			if avail <= 0 {
				avail = 1
			}

			for len(contentRunes) > 0 {
				wrapped := false
				chunk := contentRunes
				if len(chunk) > avail {
					// We can display at most (avail-1) content runes on this
					// line so that there is room for the trailing '\\'
					// marker in the last visible text column.
					chunkLen := avail - 1
					if chunkLen < 1 {
						chunkLen = 1
					}
					if chunkLen > len(chunk) {
						chunkLen = len(chunk)
					}
					chunk = chunk[:chunkLen]
					wrapped = true
				}
				textRunes := append(append([]rune{}, prefixRunes...), chunk...)
				if wrapped {
					// Append a wrap marker in the last column.
					textRunes = append(textRunes, '\\')
				}
				lines = append(lines, ui.FrameLine{Runes: textRunes, Attr: attr})

				if !wrapped {
					break
				}

				// Remaining runes for further wrapped lines. We have consumed
				// len(chunk) runes from contentRunes; the next line should
				// start immediately after the last displayed character.
				contentRunes = contentRunes[len(chunk):]
				// For continuation lines, indent to align with content.
				prefixRunes = []rune(strings.Repeat(" ", len([]rune(prefix))))
				avail = textWidth - len(prefixRunes)
				if avail <= 0 {
					avail = 1
				}
			}
		}
	}

	return lines
}
