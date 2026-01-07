/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"fmt"
	"strings"

	gc "github.com/gbin/goncurses"
)

// Modal is a thin wrapper around a Frame that provides a centered,
// boxed subwindow suitable for modal dialogs. It is intentionally
// generic: higher-level helpers (such as line-input or list-selection
// dialogs) can use Modal to obtain a bordered, centered region and then
// draw headers, input areas, and list content as needed.
//
// The existing NcursesUI modal helpers in ncurses.go (readLineModal,
// selectFromListModal, etc.) currently manage their own windows
// directly. New code should prefer using Modal + Frame instead so that
// window creation, centering, borders, and cleanup behavior are
// consistent.
type Modal struct {
	ui *NcursesUI

	// Frame is the underlying drawable region, including the border.
	Frame *Frame

	// Cached inner content geometry (relative to Frame.Win). These
	// correspond to the values returned by Frame.contentBounds() at the
	// time the modal was created.
	contentY      int
	contentX      int
	contentHeight int
	contentWidth  int
}

// newCenteredModal creates a centered modal Frame of approximately the
// requested height and width. The dimensions are clamped to fit within
// the current terminal size and to reasonable minimums so that there is
// always room for borders and at least one row/column of content.
//
// The resulting Frame always has a border (box) drawn by Render(). The
// hasCursor and hasInput flags are passed through to the Frame
// constructor.
func (n *NcursesUI) newCenteredModal(height, width int, hasCursor, hasInput bool) (*Modal, error) {
	if n == nil || n.scr == nil {
		return nil, fmt.Errorf("ncurses UI not initialized")
	}

	maxY, maxX := n.scr.MaxYX()
	if maxY < 3 || maxX < 4 {
		return nil, fmt.Errorf("terminal too small for ncurses window")
	}

	// Clamp requested geometry similarly to newCenteredBox so that
	// existing sizing behavior is preserved.
	if height < 3 {
		height = 3
	}
	if height > maxY-2 {
		height = maxY - 2
	}
	if width < 4 {
		width = 4
	}
	if width > maxX-2 {
		width = maxX - 2
	}
	if height < 3 || width < 4 {
		return nil, fmt.Errorf("terminal too small for requested window")
	}

	startY := (maxY - height) / 2
	startX := (maxX - width) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	frame, err := NewFrame(n.scr, height, width, startY, startX, true /* hasBorder */, hasCursor, hasInput)
	if err != nil {
		return nil, err
	}

	cy, cx, ch, cw := frame.contentBounds()

	m := &Modal{
		ui:            n,
		Frame:         frame,
		contentY:      cy,
		contentX:      cx,
		contentHeight: ch,
		contentWidth:  cw,
	}

	return m, nil
}

// Close deletes the modal's underlying Frame window and forces the root
// screen to be treated as fully "touched" so that a subsequent Refresh
// repaints the entire area the dialog occupied. This mirrors the
// behavior of deleteModelAndRefreshParent in ncurses.go.
func (m *Modal) Close() {
	if m == nil || m.Frame == nil || m.ui == nil || m.ui.scr == nil {
		return
	}

	// Delete the modal's frame (and its panel) so that it is removed
	// from the global panel stack. Then ask ncurses to recompute the
	// panel layout and flush the virtual screen to the physical
	// terminal. This restores the previously-covered windows (such as
	// the thread history/input frames) without requiring callers to
	// explicitly redraw them.
	m.Frame.Close()
	gc.UpdatePanels()
	m.ui.scr.Refresh()
}

// ContentArea returns the inner content rectangle of the modal relative
// to the modal's Frame window. Callers can use this to lay out prompt
// text, input rows, and list content without needing to know about the
// outer border geometry.
func (m *Modal) ContentArea() (y, x, h, w int) {
	if m == nil {
		return 0, 0, 0, 0
	}
	return m.contentY, m.contentX, m.contentHeight, m.contentWidth
}

// readLineModalFrame displays a simple centered modal window backed by a
// Frame and returns the line of user input (without a trailing
// newline). It mirrors the behavior of NcursesUI.readLineModal in
// ncurses.go but uses Modal/newCenteredModal for window management.
//
// This helper is currently unused by NcursesUI.Get, which still routes
// through the original implementation. It exists to enable a
// Frame-based migration path for modal dialogs without immediately
// changing existing behavior.
func (n *NcursesUI) readLineModalFrame(userPrompt string) (string, error) {
	if n == nil {
		return "", fmt.Errorf("ncurses UI not initialized")
	}

	// Allow multi-line prompts by splitting on explicit newlines, so
	// sizing reflects the actual layout.
	trimmed := strings.TrimRight(userPrompt, "\n")
	promptLines := strings.Split(trimmed, "\n")

	// Basic modal dimensions: borders + prompt lines + input line.
	desiredHeight := len(promptLines) + 3
	if desiredHeight < 5 {
		desiredHeight = 5
	}

	// Width is based on the longest prompt line.
	maxRunes := 0
	for _, line := range promptLines {
		if l := len([]rune(line)); l > maxRunes {
			maxRunes = l
		}
	}
	innerWidth := maxRunes + 2
	if innerWidth < 30 {
		innerWidth = 30
	}

	modal, err := n.newCenteredModal(desiredHeight, innerWidth, false /* hasCursor */, false /* hasInput */)
	if err != nil {
		return "", err
	}
	defer modal.Close()

	win := modal.Frame.Win
	cy, cx, ch, cw := modal.ContentArea()
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	// Use a short timeout so we can blink the software cursor even when the
	// user is idle (consistent with the rest of the ncurses UI).
	win.Timeout(50)
	blinkOn := true
	blinkCounter := 0
	const blinkTicks = 6 // ~300ms at 50ms timeout

	// Draw the border once; subsequent updates only modify the inner
	// content area.
	_ = win.Box(0, 0)

	// Render each prompt line starting at the first inner row. Any
	// excess lines are silently dropped if the terminal is extremely
	// small.
	for i, line := range promptLines {
		if i >= ch {
			break
		}
		win.MovePrint(cy+i, cx, TruncateRunes(line, cw))
	}

	var buf []rune
	// Place the input row directly after the last rendered prompt line,
	// clamped to the last available inner row so we stay inside the box.
	inputY := cy + len(promptLines)
	if inputY >= cy+ch {
		inputY = cy + ch - 1
	}

	for {
		// Clear input line inside the content area.
		for x := 0; x < cw; x++ {
			win.MovePrint(inputY, cx+x, " ")
		}

		// Render current buffer truncated to fit.
		inputWidth := cw
		if inputWidth < 1 {
			inputWidth = 1
		}
		text := TruncateRunes(string(buf), inputWidth)
		win.MovePrint(inputY, cx, text)

		// Draw a visible software cursor by inverting the cell at the
		// logical cursor position. We reuse the Frame cursor rendering code
		// so we don't depend on the terminal's hardware cursor visibility.
		cursorCol := len([]rune(text))
		if cursorCol < 0 {
			cursorCol = 0
		}
		if cursorCol >= cw {
			cursorCol = cw - 1
		}
		cursorX := cx + cursorCol

		underCh := ' '
		if cursorCol >= 0 && cursorCol < len([]rune(text)) {
			underCh = []rune(text)[cursorCol]
		}
		if blinkOn {
			modal.Frame.drawSoftCursor(inputY, cursorX, underCh)
		}
		win.Refresh()

		chKey := win.GetChar()
		if chKey == 0 {
			// Timeout/no key pressed: advance the blink timer.
			blinkCounter++
			if blinkCounter >= blinkTicks {
				blinkCounter = 0
				blinkOn = !blinkOn
			}
			continue
		}

		// Any keypress resets the blink so the cursor is visible while typing.
		blinkOn = true
		blinkCounter = 0

		switch chKey {
		case gc.Key(27): // ESC -> empty string
			return "", fmt.Errorf("cancelled")
		case gc.KEY_ENTER, gc.KEY_RETURN:
			return string(buf), nil
		case gc.KEY_BACKSPACE, 127, 8:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		default:
			// Accept any non-control byte (including high-bit bytes from
			// UTF-8 sequences) as literal input. goncurses/wgetch reports
			// regular characters as values in the 0-255 range and uses
			// larger values for KEY_* constants, so we clamp at 256 to
			// avoid accidentally treating special keys as text.
			if chKey >= 32 && chKey < 256 {
				buf = append(buf, rune(chKey))
			}
		}
	}
}

// selectFromListModalFrame displays a centered, scrollable list within a
// Frame-backed modal. It mirrors the behavior of NcursesUI.
// selectFromListModal in ncurses.go but uses Modal/newCenteredModal for
// window management.
//
// As with readLineModalFrame, this helper is not yet wired into the
// public NcursesUI selection methods; it exists as a migration target
// for future refactors.
func (n *NcursesUI) selectFromListModalFrame(userPrompt string, items []string, initialSelected int) (selectedIdx int, canceled bool, err error) {
	if n == nil {
		return -1, false, fmt.Errorf("ncurses UI not initialized")
	}
	if len(items) == 0 {
		return -1, false, fmt.Errorf("no items provided")
	}

	total := len(items)

	trimmed := strings.TrimRight(userPrompt, "\n")
	promptLines := strings.Split(trimmed, "\n")
	if len(promptLines) == 0 {
		promptLines = []string{""}
	}
	promptHeight := len(promptLines)

	maxRunes := 0
	for _, line := range promptLines {
		if l := len([]rune(line)); l > maxRunes {
			maxRunes = l
		}
	}
	for _, it := range items {
		if l := len([]rune(it)); l > maxRunes {
			maxRunes = l
		}
	}

	innerWidth := maxRunes + 2
	if innerWidth < 30 {
		innerWidth = 30
	}

	desiredHeight := promptHeight + 1 + len(items) + 2
	if desiredHeight < promptHeight+3 {
		desiredHeight = promptHeight + 3
	}

	modal, err := n.newCenteredModal(desiredHeight, innerWidth, false /* hasCursor */, false /* hasInput */)
	if err != nil {
		return -1, false, err
	}
	defer modal.Close()

	win := modal.Frame.Win
	cy, cx, ch, cw := modal.ContentArea()
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}

	// Draw the border once; subsequent updates only modify inner area.
	_ = win.Box(0, 0)

	// If the prompt is so tall relative to the actual window height that
	// it would push the list completely off-screen, trim the number of
	// visible prompt lines so that we always reserve at least a handful
	// of rows for the selectable list.
	const minListRows = 3
	effectiveMinListRows := minListRows
	if total < effectiveMinListRows {
		effectiveMinListRows = total
	}
	maxPromptLines := ch - (effectiveMinListRows + 1) // +1 for blank spacer
	if maxPromptLines < 1 {
		maxPromptLines = 1
	}
	if promptHeight > maxPromptLines {
		promptHeight = maxPromptLines
		promptLines = promptLines[:promptHeight]
	}

	selected := initialSelected
	if selected < 0 {
		selected = 0
	}
	if selected >= total {
		selected = total - 1
	}
	offset := 0

	for {
		viewHeight := ch - (promptHeight + 1)
		if viewHeight < 1 {
			viewHeight = 1
		}
		AdjustListViewport(total, viewHeight, &selected, &offset)

		// Clear content area.
		_ = win.AttrSet(gc.A_NORMAL)
		for row := 0; row < ch; row++ {
			y := cy + row
			win.Move(y, cx)
			win.HLine(y, cx, ' ', cw)
		}

		// Render prompt lines at the top of the content area.
		_ = win.AttrSet(gc.A_NORMAL)
		for i, line := range promptLines {
			if i >= ch {
				break
			}
			win.MovePrint(cy+i, cx, TruncateRunes(line, cw))
		}

		// Render list items within the remaining rows.
		var selectedAttr gc.Char
		if n.theme.UseColors && n.theme.SelectedPair != 0 {
			selectedAttr = gc.A_NORMAL | gc.ColorPair(n.theme.SelectedPair)
		} else {
			selectedAttr = gc.A_REVERSE | gc.A_NORMAL
		}
		normalAttr := gc.A_NORMAL
		listStartY := cy + promptHeight + 1 // one blank row after prompt
		for row := 0; row < viewHeight; row++ {
			idx := offset + row
			y := listStartY + row
			if y >= cy+ch {
				break
			}
			if idx >= total {
				continue
			}

			text := TruncateRunes(items[idx], cw)
			if idx == selected {
				_ = win.AttrSet(selectedAttr)
			} else {
				_ = win.AttrSet(normalAttr)
			}
			win.Move(y, cx)
			win.HLine(y, cx, ' ', cw)
			win.MovePrint(y, cx, text)
		}

		win.Refresh()

		chKey := win.GetChar()
		if chKey == 0 {
			continue
		}

		switch chKey {
		case gc.Key(27): // ESC -> report cancellation
			return -1, true, nil
		case gc.KEY_ENTER, gc.KEY_RETURN:
			return selected, false, nil
		case gc.KEY_UP:
			if selected > 0 {
				selected--
			}
		case gc.KEY_DOWN:
			if selected < total-1 {
				selected++
			}
		case gc.KEY_HOME:
			selected = 0
		case gc.KEY_END:
			selected = total - 1
		case gc.KEY_PAGEUP:
			if viewHeight > 0 {
				selected -= viewHeight
				if selected < 0 {
					selected = 0
				}
			}
		case gc.KEY_PAGEDOWN:
			if viewHeight > 0 {
				selected += viewHeight
				if selected > total-1 {
					selected = total - 1
				}
			}
		default:
			// Ignore other keys.
		}
	}
}
