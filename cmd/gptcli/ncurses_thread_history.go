/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"strings"

	gc "github.com/gbin/goncurses"
)

// visualLine represents a single, fully-rendered line of text in the
// thread history area after wrapping and prefixing. It carries simple
// semantic flags so the renderer can apply different colors or
// attributes for user/assistant text and code blocks.
type visualLine struct {
	text   string
	isUser bool
	isCode bool
}

// buildHistoryLines converts the logical RenderBlocks for a thread
// into a flat slice of visualLine values, applying prefixes ("You:",
// "LLM:") and soft wrapping with a trailing '\\' on wrapped
// segments. The resulting slice is suitable for direct line-by-line
// rendering in the history pane.
func buildHistoryLines(thread *GptCliThread, width int) []visualLine {
	if width <= 0 {
		return nil
	}
	blocks := thread.RenderBlocks()
	lines := make([]visualLine, 0)

	wrapWidth := width
	for _, b := range blocks {
		var prefix string
		isUser := false
		isCode := false

		switch b.Kind {
		case RenderBlockUserPrompt:
			prefix = "You: "
			isUser = true
		case RenderBlockAssistantText, RenderBlockAssistantCode:
			prefix = "LLM: "
			isUser = false
		}

		if b.Kind == RenderBlockAssistantCode {
			isCode = true
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
			avail := wrapWidth - len(prefixRunes)
			if avail <= 0 {
				avail = 1
			}

			for len(contentRunes) > 0 {
				chunk := contentRunes
				wrapped := false
				if len(chunk) > avail {
					chunk = chunk[:avail-1]
					wrapped = true
				}
				text := string(prefixRunes) + string(chunk)
				if wrapped {
					// Append a wrap marker in the last column.
					text += "\\"
				}
				lines = append(lines, visualLine{
					text:   text,
					isUser: isUser,
					isCode: isCode,
				})

				if !wrapped {
					break
				}

				// Remaining runes for further wrapped lines.
				contentRunes = contentRunes[avail-1:]
				// For continuation lines, indent to align with content.
				prefixRunes = []rune(strings.Repeat(" ", len([]rune(prefix))))
				avail = wrapWidth - len(prefixRunes)
				if avail <= 0 {
					avail = 1
				}
			}
		}
	}

	return lines
}

// drawThreadHistory draws the scrollable history pane for the current
// thread.
func drawThreadHistory(scr *gc.Window, lines []visualLine, offset int) {
	maxY, maxX := scr.MaxYX()
	startY := menuHeaderHeight
	endY := maxY - menuStatusHeight - threadInputHeight // input box above status
	if endY <= startY {
		return
	}
	height := endY - startY
	if height < 1 {
		height = 1
	}

	// Compute scrollbar layout using the shared helper. The scrollbar is
	// rendered in the last column.
	total := len(lines)
	sbX := maxX - 1
	sb := computeScrollbar(total, height, offset)

	for row := 0; row < height; row++ {
		idx := offset + row
		rowY := startY + row
		scr.Move(rowY, 0)
		scr.HLine(rowY, 0, ' ', maxX)
		if idx < len(lines) {
			vl := lines[idx]
			// Choose color/attributes based on role and code flag.
			attr := gc.A_NORMAL
			if globalUseColors {
				if vl.isCode {
					attr = gc.ColorPair(threadColorCode)
				} else if vl.isUser {
					attr = gc.ColorPair(threadColorUser)
				} else {
					attr = gc.ColorPair(threadColorAssistant)
				}
			} else {
				if vl.isCode {
					attr = gc.A_BOLD
				} else if vl.isUser {
					attr = gc.A_BOLD
				} else {
					attr = gc.A_NORMAL
				}
			}
			_ = scr.AttrSet(attr)
			// Leave the last column free for the scrollbar.
			limit := maxX
			if limit > 0 {
				limit--
			}
			text := vl.text
			runes := []rune(text)
			if len(runes) > limit {
				text = string(runes[:limit])
			}
			scr.MovePrint(rowY, 0, text)
		}

		// Draw the scrollbar track, thumb, and arrow glyphs in the last
		// column via the shared helper. When no scrollbar is needed the
		// helper becomes a no-op, and the column remains blank.
		if sbX >= 0 {
			drawScrollbarCell(scr, rowY, row, height, sbX, sb)
		}
	}
	_ = scr.AttrSet(gc.A_NORMAL)
}
