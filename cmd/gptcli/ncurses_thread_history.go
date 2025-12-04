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
// thread. When focusHistory is active it also overlays a software
// cursor at the given logical cursorLine/cursorCol, using blinkOn to
// control visibility so it can share the same blink state as the input
// cursor.
func drawThreadHistory(scr *gc.Window, lines []visualLine, offset int,
	focus threadViewFocus, cursorLine, cursorCol int, blinkOn bool) {
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

		// Software cursor for the history pane. This mirrors the input
		// cursor but leaves the history read-only: navigation keys move
		// cursorLine/cursorCol while the underlying text is not editable.
		// The cursor is only shown when the history pane has focus and
		// blinkOn is true.
		if focus == focusHistory && blinkOn && idx == cursorLine {
			cx := clampCursorX(cursorCol, maxX, true)

			// Determine the underlying rune at the cursor position so we
			// invert that cell rather than drawing a generic block. When
			// the cursor sits past the end of the text we just highlight a
			// space.
			ch := ' '
			if idx >= 0 && idx < len(lines) {
				lineRunes := []rune(lines[idx].text)
				if cursorCol >= 0 && cursorCol < len(lineRunes) {
					ch = lineRunes[cursorCol]
				}
			}

			drawSoftCursor(scr, rowY, cx, ch)
		}
	}
	_ = scr.AttrSet(gc.A_NORMAL)
}

// clampHistoryViewport normalizes the history viewport after changes to
// terminal size or content. It keeps offset and cursorLine within valid
// bounds and ensures the cursor's line is visible on screen.
func clampHistoryViewport(maxY int, lines []visualLine, offset *int, cursorLine *int) {
	startY := menuHeaderHeight
	endY := maxY - menuStatusHeight - threadInputHeight
	if endY <= startY {
		endY = startY + 1
	}
	historyHeight := endY - startY
	if historyHeight < 1 {
		historyHeight = 1
	}

	total := len(lines)
	if total == 0 {
		*offset = 0
		*cursorLine = 0
		return
	}

	if *cursorLine < 0 {
		*cursorLine = 0
	}
	if *cursorLine > total-1 {
		*cursorLine = total - 1
	}

	maxOffset := total - historyHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if *offset < 0 {
		*offset = 0
	}
	if *offset > maxOffset {
		*offset = maxOffset
	}

	// Ensure the cursor's line is visible within the viewport.
	if *cursorLine < *offset {
		*offset = *cursorLine
	} else if *cursorLine >= *offset+historyHeight {
		*offset = *cursorLine - historyHeight + 1
	}
	if *offset < 0 {
		*offset = 0
	}
	if *offset > maxOffset {
		*offset = maxOffset
	}
}

