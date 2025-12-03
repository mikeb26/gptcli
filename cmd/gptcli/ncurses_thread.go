/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	gc "github.com/gbin/goncurses"

	"github.com/mikeb26/gptcli/internal/ui"
)

const (
	// Additional color pairs for the thread view. These are initialized
	// alongside the menu colors in initUI so they can be reused by any
	// ncurses-based views.
	threadColorUser      int16 = 5
	threadColorAssistant int16 = 6
	threadColorCode      int16 = 7
)

// threadViewFocus tracks which pane is currently active inside the
// thread view. This determines how keys are interpreted (e.g. whether
// 'q' quits the view or is inserted into the input buffer).
type threadViewFocus int

const (
	focusHistory threadViewFocus = iota
	focusInput
)

// drawThreadStatus renders a simple status line at the bottom of the
// screen, including mode information and key hints.
func drawThreadStatus(scr *gc.Window, focus threadViewFocus, msg string) {
	maxY, maxX := scr.MaxYX()
	statusY := maxY - 1
	if statusY < 0 {
		return
	}

	label := "Hist"
	if focus == focusInput {
		label = "Input"
	}

	if msg == "" {
		msg = "Tab: switch  Ctrl-D: send  ESC: back  q: quit"
	}

	full := fmt.Sprintf("[%s] %s", label, msg)
	if len([]rune(full)) > maxX {
		full = string([]rune(full)[:maxX])
	}

	drawStatusBar(scr, statusY, full, globalUseColors)
}

// drawThreadHeader renders a single-line header for the thread view.
func drawThreadHeader(scr *gc.Window, thread *GptCliThread) {
	maxY, maxX := scr.MaxYX()
	if maxY <= 0 {
		return
	}
	header := fmt.Sprintf("Thread: %s", thread.Name)
	if len([]rune(header)) > maxX {
		header = string([]rune(header)[:maxX])
	}

	var attr gc.Char = gc.A_BOLD
	if globalUseColors {
		attr = gc.A_BOLD | gc.ColorPair(menuColorHeader)
	}
	_ = scr.AttrSet(attr)
	scr.Move(0, 0)
	scr.HLine(0, 0, ' ', maxX)
	scr.MovePrint(0, 0, header)
	_ = scr.AttrSet(gc.A_NORMAL)
}

// runThreadView provides an ncurses-based view for interacting with a
// single thread. It renders the existing dialogue and allows the user
// to enter a multi-line prompt in a 3-line input box. Ctrl-D sends the
// current input buffer via ChatOnceInCurrentThread. History and input
// areas are independently scrollable via focus switching (Tab) and
// standard navigation keys. Pressing 'q' or ESC in the history focus
// returns to the menu.
func runThreadView(ctx context.Context, scr *gc.Window, gptCliCtx *GptCliContext, thread *GptCliThread) error { //nolint:revive,unused
	// Listen for SIGWINCH so we can adjust layout on resize while inside
	// the thread view. This mirrors the behavior of showMenu but keeps
	// all ncurses calls confined to this goroutine.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	maxY, maxX := scr.MaxYX()
	ncui := ui.NewNcursesUI(scr)
	historyLines := buildHistoryLines(thread, maxX)
	historyOffset := 0
	input := &inputState{}
	input.reset()
	focus := focusInput
	needRedraw := true

	// Simple blink state for the software cursor in the input area. We
	// toggle blinkOn after a small number of input polling ticks so it
	// blinks even when the user is idle.
	blinkOn := true
	blinkCounter := 0
	const blinkTicks = 6 // ~300ms at the menu's 50ms timeout

	for {
		if needRedraw {
			scr.Erase()
			drawThreadHeader(scr, thread)
			drawThreadHistory(scr, historyLines, historyOffset)
			drawThreadInput(scr, input, focus, blinkOn)
			drawThreadStatus(scr, focus, "")
			scr.Refresh()
			needRedraw = false
		}

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(scr)
			maxY, maxX = scr.MaxYX()
			historyLines = buildHistoryLines(thread, maxX)
			if historyOffset < 0 {
				historyOffset = 0
			}
			needRedraw = true
			continue
		default:
			ch = scr.GetChar()
			if ch == 0 {
				// Timeout/no key pressed: advance the blink timer for the
				// software cursor in the input area.
				blinkCounter++
				if blinkCounter >= blinkTicks {
					blinkCounter = 0
					blinkOn = !blinkOn
					if focus == focusInput {
						needRedraw = true
					}
				}
				continue
			}
		}

		// Compute history view height for scrolling calculations.
		startY := menuHeaderHeight
		endY := maxY - menuStatusHeight - threadInputHeight
		if endY <= startY {
			endY = startY + 1
		}
		historyHeight := endY - startY

		switch focus {
		case focusHistory:
			switch ch {
			case 'q', 'Q', 'd' - 'a' + 1, gc.Key(27): // q/Q, ctrl-d, ESC
				return nil
			case gc.KEY_UP:
				if historyOffset > 0 {
					historyOffset--
					needRedraw = true
				}
			case gc.KEY_DOWN:
				if historyOffset+historyHeight < len(historyLines) {
					historyOffset++
					needRedraw = true
				}
			case gc.KEY_PAGEUP:
				if historyHeight > 0 {
					historyOffset -= historyHeight
					if historyOffset < 0 {
						historyOffset = 0
					}
					needRedraw = true
				}
			case gc.KEY_PAGEDOWN:
				if historyHeight > 0 {
					historyOffset += historyHeight
					if historyOffset+historyHeight > len(historyLines) {
						historyOffset = len(historyLines) - historyHeight
						if historyOffset < 0 {
							historyOffset = 0
						}
					}
					needRedraw = true
				}
			case gc.KEY_HOME:
				historyOffset = 0
				needRedraw = true
			case gc.KEY_END:
				if historyHeight > 0 {
					historyOffset = len(historyLines) - historyHeight
					if historyOffset < 0 {
						historyOffset = 0
					}
					needRedraw = true
				}
			case gc.KEY_RESIZE:
				resizeScreen(scr)
				maxY, maxX = scr.MaxYX()
				historyLines = buildHistoryLines(thread, maxX)
				needRedraw = true
			case gc.KEY_TAB:
				focus = focusInput
				needRedraw = true
			}
		case focusInput:
			switch ch {
			case gc.KEY_RESIZE:
				resizeScreen(scr)
				maxY, maxX = scr.MaxYX()
				historyLines = buildHistoryLines(thread, maxX)
				needRedraw = true
			case gc.KEY_TAB:
				focus = focusHistory
				needRedraw = true
			case gc.Key(27): // ESC
				focus = focusHistory
				needRedraw = true
			case gc.KEY_HOME:
				// Move to the very beginning of the input buffer (first
				// character of the first line), mirroring HOME behavior in
				// the history view.
				input.cursorLine = 0
				input.cursorCol = 0
				input.scroll = 0
				needRedraw = true
			case gc.KEY_END:
				// Move to the very end of the input buffer (last character
				// of the last line), mirroring END behavior in the history
				// view.
				if len(input.lines) > 0 {
					input.cursorLine = len(input.lines) - 1
					input.cursorCol = len(input.lines[input.cursorLine])
					// Ensure the last line is visible; adjust scroll based on
					// the height of the input area.
					visible := threadInputHeight - 1
					if visible < 1 {
						visible = 1
					}
					maxScroll := len(input.lines) - visible
					if maxScroll < 0 {
						maxScroll = 0
					}
					if input.cursorLine < input.scroll {
						input.scroll = input.cursorLine
					} else if input.cursorLine >= input.scroll+visible {
						input.scroll = input.cursorLine - visible + 1
					}
					if input.scroll > maxScroll {
						input.scroll = maxScroll
					}
					if input.scroll < 0 {
						input.scroll = 0
					}
				}
				needRedraw = true
			case gc.KEY_PAGEUP:
				// Scroll and move the cursor up by one visible page.
				visible := threadInputHeight - 1
				if visible < 1 {
					visible = 1
				}
				input.cursorLine -= visible
				if input.cursorLine < 0 {
					input.cursorLine = 0
				}
				input.scroll -= visible
				if input.scroll < 0 {
					input.scroll = 0
				}
				if input.cursorLine < input.scroll {
					input.scroll = input.cursorLine
				}
				if input.cursorLine >= 0 && input.cursorLine < len(input.lines) && input.cursorCol > len(input.lines[input.cursorLine]) {
					input.cursorCol = len(input.lines[input.cursorLine])
				}
				needRedraw = true
			case gc.KEY_PAGEDOWN:
				// Scroll and move the cursor down by one visible page.
				visible := threadInputHeight - 1
				if visible < 1 {
					visible = 1
				}
				input.cursorLine += visible
				if input.cursorLine > len(input.lines)-1 {
					input.cursorLine = len(input.lines) - 1
				}
				maxScroll := len(input.lines) - visible
				if maxScroll < 0 {
					maxScroll = 0
				}
				input.scroll += visible
				if input.scroll > maxScroll {
					input.scroll = maxScroll
				}
				if input.cursorLine >= input.scroll+visible {
					input.scroll = input.cursorLine - visible + 1
				}
				if input.cursorLine >= 0 && input.cursorLine < len(input.lines) && input.cursorCol > len(input.lines[input.cursorLine]) {
					input.cursorCol = len(input.lines[input.cursorLine])
				}
				needRedraw = true
			case gc.KEY_LEFT:
				input.moveCursorLeft()
				needRedraw = true
			case gc.KEY_RIGHT:
				input.moveCursorRight()
				needRedraw = true
			case gc.KEY_UP:
				input.moveCursorUp()
				if input.cursorLine < input.scroll {
					input.scroll = input.cursorLine
				}
				needRedraw = true
			case gc.KEY_DOWN:
				input.moveCursorDown()
				visible := threadInputHeight - 1
				if visible < 1 {
					visible = 1
				}
				if input.cursorLine >= input.scroll+visible {
					input.scroll = input.cursorLine - visible + 1
				}
				needRedraw = true
			case gc.KEY_BACKSPACE, 127, 8:
				input.backspace()
				if input.cursorLine < input.scroll {
					input.scroll = input.cursorLine
				}
				needRedraw = true
			case gc.KEY_ENTER, gc.KEY_RETURN:
				input.insertNewline()
				visible := threadInputHeight - 1
				if visible < 1 {
					visible = 1
				}
				if input.cursorLine >= input.scroll+visible {
					input.scroll = input.cursorLine - visible + 1
				}
				needRedraw = true
			case 'd' - 'a' + 1: // Ctrl-D sends the input buffer
				prompt := strings.TrimSpace(input.toString())
				if prompt == "" {
					continue
				}
				// Show processing status
				drawThreadStatus(scr, focus, "Processing...")
				scr.Refresh()

				retry := true
				for retry {
					_, err := gptCliCtx.ChatOnceInCurrentThread(ctx, prompt)
					if err == nil {
						retry = false
						break
					}

					// Show error modal asking whether to retry.
					wantRetry, modalErr := showErrorRetryModal(ncui, err.Error())
					if modalErr != nil || !wantRetry {
						retry = false
						break
					}
				}

				// Refresh thread data from the updated current thread.
				maxY, maxX = scr.MaxYX()
				historyLines = buildHistoryLines(thread, maxX)
				if len(historyLines) > historyHeight {
					historyOffset = len(historyLines) - historyHeight
				} else {
					historyOffset = 0
				}

				// Clear input buffer on success or after giving up.
				input.reset()
				needRedraw = true
			default:
				// Treat any printable byte (including high‑bit bytes from
				// UTF‑8 sequences) as input. When running in a UTF-8
				// locale, ncurses/GetChar returns each byte of the sequence
				// separately; group those bytes into a single rune so that
				// characters like emoji render correctly.
				if ch >= 32 && ch < 256 {
					r := readUTF8KeyRune(scr, ch)
					input.insertRune(r)
					needRedraw = true
				}
			}
		}
	}
}
