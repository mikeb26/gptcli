/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/famz/SetLocale"
	gc "github.com/gbin/goncurses"
	"golang.org/x/term"
)

const (
	menuHeaderHeight         = 1
	menuStatusHeight         = 1
	menuColorHeader    int16 = 1
	menuColorStatus    int16 = 2
	menuColorSelected  int16 = 3
	menuColorStatusKey int16 = 4

	// Additional color pairs for the thread view. These are initialized
	// alongside the menu colors in initUI so they can be reused by any
	// ncurses-based views.
	threadColorUser      int16 = 5
	threadColorAssistant int16 = 6
	threadColorCode      int16 = 7
)

// globalUseColors mirrors the color capability detected in initUI so
// that other ncurses views (like the per-thread view) can make the
// same color vs monochrome decisions without re-detecting.
var globalUseColors bool

// threadViewFocus tracks which pane is currently active inside the
// thread view. This determines how keys are interpreted (e.g. whether
// 'q' quits the view or is inserted into the input buffer).
type threadViewFocus int

const (
	focusHistory threadViewFocus = iota
	focusInput
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

// inputState holds the editable multi-line input buffer used in the
// thread view, along with cursor and scroll position.
type inputState struct {
	lines      [][]rune
	cursorLine int
	cursorCol  int
	scroll     int // first visible logical line index in the input area
}

type threadMenuUI struct {
	scr       *gc.Window
	items     []string
	selected  int
	offset    int
	useColors bool
}

func newThreadMenuUI(scr *gc.Window, useColors bool) *threadMenuUI {
	return &threadMenuUI{
		scr:       scr,
		items:     make([]string, 0),
		selected:  0,
		offset:    0,
		useColors: useColors,
	}
}

func (ui *threadMenuUI) viewHeight() int {
	maxY, _ := ui.scr.MaxYX()
	vh := maxY - menuHeaderHeight - menuStatusHeight
	if vh < 0 {
		return 0
	}
	return vh
}

func (ui *threadMenuUI) adjustOffset() {
	vh := ui.viewHeight()
	total := len(ui.items)
	if vh <= 0 || total == 0 {
		ui.offset = 0
		if total == 0 {
			ui.selected = 0
		} else if ui.selected >= total {
			ui.selected = total - 1
		} else if ui.selected < 0 {
			ui.selected = 0
		}
		return
	}

	if ui.selected < 0 {
		ui.selected = 0
	}
	if ui.selected >= total {
		ui.selected = total - 1
	}

	if ui.offset > ui.selected {
		ui.offset = ui.selected
	}
	if ui.selected >= ui.offset+vh {
		ui.offset = ui.selected - vh + 1
	}

	maxOffset := total - vh
	if maxOffset < 0 {
		maxOffset = 0
	}
	if ui.offset > maxOffset {
		ui.offset = maxOffset
	}
	if ui.offset < 0 {
		ui.offset = 0
	}
}

// truncateToWidth returns a prefix of s that fits in max cells, treating
// the string as UTF-8 and counting runes instead of bytes. This avoids
// splitting multi-byte UTF-8 sequences when we need to clamp text to the
// current terminal width. It assumes that each rune occupies a single
// column cell, which holds for the common box-drawing and arrow glyphs
// used in the menu UI.
func truncateToWidth(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func (ui *threadMenuUI) draw() {
	scr := ui.scr

	maxY, maxX := scr.MaxYX()
	vh := ui.viewHeight()

	ui.adjustOffset()

	headerTitle := strings.Split(threadGroupHeaderString(false), "\n")[0]
	headerTitle = truncateToWidth(headerTitle, maxX)

	if ui.useColors {
		_ = scr.AttrSet(gc.A_BOLD | gc.ColorPair(menuColorHeader))
	} else {
		_ = scr.AttrSet(gc.A_BOLD)
	}
	scr.Move(0, 0)
	scr.HLine(0, 0, ' ', maxX)
	scr.MovePrintf(0, 0, "%s", headerTitle)

	// Scrollable list area
	startY := menuHeaderHeight
	if vh > 0 && len(ui.items) > 0 {
		for row := 0; row < vh; row++ {
			idx := ui.offset + row
			if idx >= len(ui.items) {
				break
			}
			line := truncateToWidth(ui.items[idx], maxX)

			if idx == ui.selected {
				if ui.useColors {
					_ = scr.AttrSet(gc.A_BOLD | gc.ColorPair(menuColorSelected))
				} else {
					_ = scr.AttrSet(gc.A_REVERSE | gc.A_BOLD)
				}
			} else {
				_ = scr.AttrSet(gc.A_NORMAL)
			}

			// Fill the entire row so the background spans full width, then
			// render the visible text at the start of the line.
			rowY := startY + row
			scr.Move(rowY, 0)
			scr.HLine(rowY, 0, ' ', maxX)
			scr.MovePrintf(rowY, 0, "%s", line)
		}
	}

	// Status bar at bottom
	_ = scr.AttrSet(gc.A_NORMAL)
	statusY := maxY - 1
	if statusY >= 0 {
		// Build a status line where key bindings are visually highlighted
		// (bold) while keeping a consistent background color or reverse
		// video across the entire bar.
		segments := []struct {
			text string
			bold bool
		}{
			{"Nav:", false},
			{"↑", true},
			{"/", false},
			{"↓", true},
			{"/", false},
			{"PgUp", true},
			{"/", false},
			{"PgDn", true},
			{"/", false},
			{"Home", true},
			{"/", false},
			{"End", true},
			{" Sel:", false},
			{"⏎", true},
			{" New:", false},
			{"n", true},
			{" Archive:", false},
			{"a", true},
			{" Quit:", false},
			{"q", true},
		}

		// Base attributes for the status bar background. Use the goncurses
		// Char type explicitly so it matches the AttrSet signature.
		var baseAttr gc.Char = gc.A_REVERSE
		if ui.useColors {
			baseAttr = gc.ColorPair(menuColorStatus)
		}
		_ = scr.AttrSet(baseAttr)

		// Clear the status line first so the background spans the width.
		scr.Move(statusY, 0)
		scr.HLine(statusY, 0, ' ', maxX)

		// Render each segment, enabling bold (and red foreground when colors
		// are active) only for the key tokens.
		x := 0
		for _, seg := range segments {
			if x >= maxX {
				break
			}
			if seg.bold {
				if ui.useColors {
					_ = scr.AttrSet(gc.ColorPair(menuColorStatusKey))
				} else {
					_ = scr.AttrOn(gc.A_BOLD)
				}
			} else {
				_ = scr.AttrSet(baseAttr)
			}

			remaining := maxX - x
			if remaining <= 0 {
				break
			}
			runes := []rune(seg.text)
			if len(runes) > remaining {
				runes = runes[:remaining]
			}
			text := string(runes)

			scr.MovePrint(statusY, x, text)
			x += len(runes)
		}
	}

	_ = scr.AttrSet(gc.A_NORMAL)
	scr.Refresh()
}

func initUI(scr *gc.Window, menuText string) (*threadMenuUI, error) {
	gc.CBreak(true)
	gc.Echo(false)
	_ = gc.Cursor(0)
	_ = scr.Keypad(true)
	scr.Timeout(50)

	// Initialize colors if available
	useColors := false
	err := gc.StartColor()
	if err == nil {
		err = gc.UseDefaultColors()
	}
	if err == nil {
		errH := gc.InitPair(menuColorHeader, gc.C_BLACK, gc.C_GREEN)
		errS := gc.InitPair(menuColorStatus, gc.C_BLACK, gc.C_CYAN)
		errSel := gc.InitPair(menuColorSelected, gc.C_BLACK, gc.C_CYAN)
		errStatusKey := gc.InitPair(menuColorStatusKey, gc.C_RED, gc.C_CYAN)
		if errH == nil && errS == nil && errSel == nil && errStatusKey == nil {
			// Initialize additional color pairs used by the per-thread
			// view. If any of these fail we still keep the base menu
			// colors active and fall back to monochrome styling within
			// the thread view for the affected roles.
			_ = gc.InitPair(threadColorUser, gc.C_YELLOW, -1)
			_ = gc.InitPair(threadColorAssistant, gc.C_CYAN, -1)
			_ = gc.InitPair(threadColorCode, gc.C_GREEN, -1)

			useColors = true
		}
	}

	ui := newThreadMenuUI(scr, useColors)
	// Record color capability for use by the thread view and any other
	// ncurses-based screens.
	globalUseColors = useColors
	ui.resetItems(menuText)

	return ui, nil
}

// promptForThreadNameNCurses displays a simple centered modal window asking
// the user to enter a new thread name. It returns the entered string (with
// surrounding whitespace trimmed) or an empty string if the user cancels
// with ESC. All interaction happens via ncurses so it is safe to call while
// the main menu UI is active.
func promptForThreadNameNCurses(scr *gc.Window) (string, error) {
	maxY, maxX := scr.MaxYX()

	// Basic modal dimensions
	height := 5
	width := 50
	if width > maxX-2 {
		width = maxX - 2
	}
	if height > maxY-2 {
		height = maxY - 2
	}
	startY := (maxY - height) / 2
	startX := (maxX - width) / 2

	win, err := gc.NewWindow(height, width, startY, startX)
	if err != nil {
		return "", err
	}
	defer win.Delete()

	win.Keypad(true)
	win.Box(0, 0)
	prompt := "Enter new thread name (ESC to cancel):"
	if len([]rune(prompt)) > width-2 {
		prompt = string([]rune(prompt)[:width-2])
	}
	win.MovePrint(1, 1, prompt)

	var buf []rune
	cursorX := 1
	inputY := 2
	for {
		// Clear input line inside the box area
		for x := 1; x < width-1; x++ {
			win.MoveAddChar(inputY, x, ' ')
		}
		// Render current buffer
		text := truncateToWidth(string(buf), width-2)
		win.MovePrint(inputY, 1, text)
		cursorX = 1 + len([]rune(text))
		if cursorX >= width-1 {
			cursorX = width - 2
		}
		win.Move(inputY, cursorX)
		win.Refresh()

		ch := win.GetChar()
		if ch == 0 {
			continue
		}

		switch ch {
		case gc.Key(27): // ESC
			return "", nil
		case gc.KEY_ENTER, gc.KEY_RETURN:
			name := strings.TrimSpace(string(buf))
			if name == "" {
				// Ignore empty name, keep prompting
				continue
			}
			return name, nil
		case gc.KEY_BACKSPACE, 127, 8:
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
			}
		default:
			// Only accept printable ASCII / UTF-8 runes
			if ch >= 32 && ch < 127 {
				buf = append(buf, rune(ch))
			}
		}
	}
}

// showErrorRetryModal displays a centered modal box with the provided
// error message and asks the user whether to retry the last
// operation. It returns true if the user chooses to retry.
func showErrorRetryModal(scr *gc.Window, message string) (bool, error) {
	maxY, maxX := scr.MaxYX()
	height := 7
	width := 60
	if width > maxX-2 {
		width = maxX - 2
	}
	if height > maxY-2 {
		height = maxY - 2
	}
	startY := (maxY - height) / 2
	startX := (maxX - width) / 2

	win, err := gc.NewWindow(height, width, startY, startX)
	if err != nil {
		return false, err
	}
	defer win.Delete()

	win.Keypad(true)
	win.Box(0, 0)

	title := "Error"
	if len([]rune(title)) > width-2 {
		title = string([]rune(title)[:width-2])
	}
	win.MovePrint(1, 2, title)

	// Render the error message trimmed to a single line inside the box.
	msgRunes := []rune(message)
	if len(msgRunes) > width-4 {
		msgRunes = msgRunes[:width-4]
	}
	win.MovePrint(2, 2, string(msgRunes))

	prompt := "Retry? (y/n)"
	if len([]rune(prompt)) > width-2 {
		prompt = string([]rune(prompt)[:width-2])
	}
	win.MovePrint(4, 2, prompt)
	win.Refresh()

	for {
		ch := win.GetChar()
		if ch == 0 {
			continue
		}
		switch ch {
		case 'y', 'Y':
			return true, nil
		case 'n', 'N', gc.Key(27): // ESC
			return false, nil
		}
	}
}

func (ui *threadMenuUI) resetItems(menuText string) error {
	trimmed := strings.TrimRight(menuText, "\n")
	if trimmed == "" {
		return fmt.Errorf("empty menu text")
	}

	ui.items = strings.Split(trimmed, "\n")

	return nil
}

func showMenu(ctx context.Context, gptCliCtx *GptCliContext, menuText string) error {
	// Require a real TTY; ncurses UI is not supported otherwise
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Errorf("menu: requires a terminal (TTY)")
	}

	SetLocale.SetLocale(SetLocale.LC_ALL, "en_US.UTF-8")
	//LC_ALL="en_US.UTF-8"	os.Setenv("LANG", )
	os.Setenv("LC_ALL", "en_US.UTF-8")
	scr, err := gc.Init()
	if err != nil {
		return fmt.Errorf("Failed to initialize screen: %w", err)
	}
	defer gc.End()

	// Listen for SIGWINCH (terminal resize). We handle the signal in this
	// same goroutine by polling the channel inside the UI loop, which
	// keeps all ncurses interaction single-threaded.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	ui, err := initUI(scr, menuText)
	needErase := true

	for {
		if needErase {
			scr.Erase()
			needErase = false
		}

		ui.draw()

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(scr)
			needErase = true
			continue
		default:
			ch = scr.GetChar()
			if ch == 0 {
				continue
			}
		}

		switch ch {
		case 'q', 'Q', 'd' - 'a' + 1: // q/Q, ctrl-d
			return nil
		case gc.KEY_UP:
			if ui.selected > 0 {
				ui.selected--
			}
		case gc.KEY_DOWN:
			if ui.selected < len(ui.items)-1 {
				ui.selected++
			}
		case gc.KEY_HOME:
			ui.selected = 0
		case gc.KEY_END:
			if len(ui.items) > 0 {
				ui.selected = len(ui.items) - 1
			}
		case gc.KEY_PAGEUP:
			if vh := ui.viewHeight(); vh > 0 {
				ui.selected -= vh
				if ui.selected < 0 {
					ui.selected = 0
				}
			}
		case gc.KEY_PAGEDOWN:
			if vh := ui.viewHeight(); vh > 0 {
				ui.selected += vh
				if ui.selected > len(ui.items)-1 {
					ui.selected = len(ui.items) - 1
				}
			}
		case gc.KEY_ENTER, gc.KEY_RETURN:
				// Enter: activate the selected thread and transition into the
				// ncurses-based thread view instead of dropping back to the
				// basic CLI prompt.
				if len(ui.items) == 0 {
					continue
				}
				threadIndex := ui.selected + 1 // threads are 1-based
				gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
				thread, err := gptCliCtx.mainThreadGroup.activateThread(threadIndex)
				if err != nil {
					// Propagate the error so the caller can handle it and
					// exit ncurses cleanly.
					return err
				}
				// Run the (stub) ncurses thread view. For now this does not yet
				// implement full in-menu interaction, but it keeps all
				// interaction within the ncurses UI instead of falling back to
				// the basic CLI prompt.
				if err := runThreadView(ctx, scr, gptCliCtx, thread); err != nil {
					return err
				}
				// After returning from the thread view, redraw the menu.
				needErase = true
		case 'n', 'N':
			needErase = true
			// Create a new thread (equivalent to the "new" subcommand), but
			// prompt for the name using an ncurses modal so we don't mix
			// stdio with the UI.
			name, err := promptForThreadNameNCurses(scr)
			if err != nil {
				return fmt.Errorf("gptcli: failed to prompt for new thread name: %w", err)
			}
			if name == "" { // user cancelled
				continue
			}
			if err := createNewThread(gptCliCtx, name); err != nil {
				return fmt.Errorf("gptcli: failed to create new thread from menu: %w", err)
			}

			// Refresh the menu items from the updated main thread group.
			if err := ui.resetItems(gptCliCtx.mainThreadGroup.String(false, false)); err != nil {
				return err
			}
			if ui.selected >= len(ui.items) {
				ui.selected = len(ui.items) - 1
			}
		case 'a':
			needErase = true
			// Archive the currently selected thread from the main thread group.
			// This mirrors the behavior of archiveThreadMain(), but uses the
			// selection from the menu UI instead of parsing a CLI argument.
			if len(ui.items) == 0 {
				continue
			}
			threadIndex := ui.selected + 1 // threads are 1-based

			// Only main-thread-group entries are shown in the menu, so we move
			// from mainThreadGroup to archiveThreadGroup directly.
			if gptCliCtx.mainThreadGroup.totThreads == 0 {
				continue
			}
			if threadIndex > gptCliCtx.mainThreadGroup.totThreads {
				continue
			}

			if err := gptCliCtx.mainThreadGroup.moveThread(threadIndex, gptCliCtx.archiveThreadGroup); err != nil {
				return fmt.Errorf("gptcli: failed to archive thread from menu: %w", err)
			}

			// Refresh the menu items from the updated main thread group.
			ui.resetItems(gptCliCtx.mainThreadGroup.String(false, false))
			if ui.selected >= len(ui.items) {
				ui.selected = len(ui.items) - 1
			}
		case gc.KEY_RESIZE:
			resizeScreen(scr)
			needErase = true
			continue
		}
	}

	// Unreachable
	// nolint
	return fmt.Errorf("BUG: unreachable")
}

// Helper to synchronize ncurses' idea of the terminal size with the
// actual TTY size. This uses golang.org/x/term to query the real
// dimensions, then asks ncurses (via goncurses) to resize its
// internal structures. All ncurses calls stay on this goroutine to
// avoid concurrency issues with C state.
func resizeScreen(scr *gc.Window) {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return
	}

	if !gc.IsTermResized(rows, cols) {
		return
	}

	_ = gc.ResizeTerm(rows, cols)
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

// reset recomputes the internal representation of the input buffer
// for a fresh, empty state.
func (st *inputState) reset() {
	st.lines = [][]rune{{}}
	st.cursorLine = 0
	st.cursorCol = 0
	st.scroll = 0
}

// insertRune inserts r at the current cursor position.
func (st *inputState) insertRune(r rune) {
	line := st.lines[st.cursorLine]
	if st.cursorCol < 0 {
		st.cursorCol = 0
	}
	if st.cursorCol > len(line) {
		st.cursorCol = len(line)
	}
	line = append(line[:st.cursorCol], append([]rune{r}, line[st.cursorCol:]...)...)
	st.lines[st.cursorLine] = line
	st.cursorCol++
}

// insertNewline splits the current line at the cursor into two lines.
func (st *inputState) insertNewline() {
	line := st.lines[st.cursorLine]
	if st.cursorCol < 0 {
		st.cursorCol = 0
	}
	if st.cursorCol > len(line) {
		st.cursorCol = len(line)
	}
	before := append([]rune{}, line[:st.cursorCol]...)
	after := append([]rune{}, line[st.cursorCol:]...)

	newLines := make([][]rune, 0, len(st.lines)+1)
	newLines = append(newLines, st.lines[:st.cursorLine]...)
	newLines = append(newLines, before)
	newLines = append(newLines, after)
	newLines = append(newLines, st.lines[st.cursorLine+1:]...)
	st.lines = newLines
	st.cursorLine++
	st.cursorCol = 0
}

// backspace removes the rune before the cursor, joining lines as needed.
func (st *inputState) backspace() {
	if st.cursorLine == 0 && st.cursorCol == 0 {
		return
	}
	line := st.lines[st.cursorLine]
	if st.cursorCol > 0 {
		if st.cursorCol > len(line) {
			st.cursorCol = len(line)
		}
		line = append(line[:st.cursorCol-1], line[st.cursorCol:]...)
		st.lines[st.cursorLine] = line
		st.cursorCol--
		return
	}
	// At column 0, join with previous line.
	prevLine := st.lines[st.cursorLine-1]
	newCol := len(prevLine)
	joined := append(append([]rune{}, prevLine...), line...)
	newLines := make([][]rune, 0, len(st.lines)-1)
	newLines = append(newLines, st.lines[:st.cursorLine-1]...)
	newLines = append(newLines, joined)
	newLines = append(newLines, st.lines[st.cursorLine+1:]...)
	st.lines = newLines
	st.cursorLine--
	st.cursorCol = newCol
}

// moveCursorLeft moves the cursor one position to the left, possibly
// wrapping to the previous line.
func (st *inputState) moveCursorLeft() {
	if st.cursorCol > 0 {
		st.cursorCol--
		return
	}
	if st.cursorLine > 0 {
		st.cursorLine--
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// moveCursorRight moves the cursor one position to the right, possibly
// wrapping to the next line.
func (st *inputState) moveCursorRight() {
	line := st.lines[st.cursorLine]
	if st.cursorCol < len(line) {
		st.cursorCol++
		return
	}
	if st.cursorLine < len(st.lines)-1 {
		st.cursorLine++
		st.cursorCol = 0
	}
}

// moveCursorUp moves the cursor one line up, keeping the closest
// horizontal column.
func (st *inputState) moveCursorUp() {
	if st.cursorLine == 0 {
		return
	}
	st.cursorLine--
	if st.cursorCol > len(st.lines[st.cursorLine]) {
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// moveCursorDown moves the cursor one line down, keeping the closest
// horizontal column.
func (st *inputState) moveCursorDown() {
	if st.cursorLine >= len(st.lines)-1 {
		return
	}
	st.cursorLine++
	if st.cursorCol > len(st.lines[st.cursorLine]) {
		st.cursorCol = len(st.lines[st.cursorLine])
	}
}

// toString flattens the multi-line input buffer to a single string.
func (st *inputState) toString() string {
	parts := make([]string, len(st.lines))
	for i, l := range st.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

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

	var attr gc.Char = gc.A_REVERSE
	if globalUseColors {
		attr = gc.ColorPair(menuColorStatus)
	}
	_ = scr.AttrSet(attr)
	scr.Move(statusY, 0)
	scr.HLine(statusY, 0, ' ', maxX)
	scr.MovePrint(statusY, 0, full)
	_ = scr.AttrSet(gc.A_NORMAL)
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

// drawThreadHistory draws the scrollable history pane for the current
// thread.
func drawThreadHistory(scr *gc.Window, lines []visualLine, offset int) {
	maxY, maxX := scr.MaxYX()
	startY := menuHeaderHeight
	endY := maxY - menuStatusHeight - 3 // 3-line input box above status
	if endY <= startY {
		return
	}
	height := endY - startY

	for row := 0; row < height; row++ {
		idx := offset + row
		rowY := startY + row
		scr.Move(rowY, 0)
		scr.HLine(rowY, 0, ' ', maxX)
		if idx >= len(lines) {
			continue
		}

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
		text := vl.text
		runes := []rune(text)
		if len(runes) > maxX {
			text = string(runes[:maxX])
		}
		scr.MovePrint(rowY, 0, text)
	}
	_ = scr.AttrSet(gc.A_NORMAL)
}

// drawThreadInput renders the 3-line input box above the status bar
// and positions the cursor according to the current inputState.
func drawThreadInput(scr *gc.Window, st *inputState, focus threadViewFocus) {
	maxY, maxX := scr.MaxYX()
	inputHeight := 3
	startY := maxY - menuStatusHeight - inputHeight
	if startY < menuHeaderHeight {
		startY = menuHeaderHeight
	}
	endY := startY + inputHeight
	label := "Input"
	if focus == focusInput {
		label = "Input*"
	}

	// Clear area
	for y := startY; y < endY; y++ {
		scr.Move(y, 0)
		scr.HLine(y, 0, ' ', maxX)
	}

	_ = scr.AttrSet(gc.A_BOLD)
	labelText := fmt.Sprintf("[%s]", label)
	if len([]rune(labelText)) > maxX {
		labelText = string([]rune(labelText)[:maxX])
	}
	scr.MovePrint(startY, 0, labelText)
	_ = scr.AttrSet(gc.A_NORMAL)

	// Render logical lines into the remaining 2 rows (or 3 if label is inline).
	visibleLines := st.lines
	if st.scroll > 0 && st.scroll < len(visibleLines) {
		visibleLines = visibleLines[st.scroll:]
	}

	// For simplicity, map each logical line to a single visual row with
	// soft truncation. This keeps input editing predictable while still
	// supporting multi-line prompts.
	for i := 0; i < inputHeight-1 && i < len(visibleLines); i++ {
		rowY := startY + 1 + i
		text := string(visibleLines[i])
		runes := []rune(text)
		if len(runes) > maxX {
			// Indicate wrap with a trailing '\\'.
			if maxX > 1 {
				text = string(runes[:maxX-1]) + "\\"
			} else {
				text = string(runes[:maxX])
			}
		}
		scr.MovePrint(rowY, 0, text)
	}

	// Position the cursor if we are in input focus.
	if focus == focusInput {
		cy := startY + 1 + (st.cursorLine - st.scroll)
		if cy < startY+1 {
			cy = startY + 1
		}
		if cy >= endY {
			cy = endY - 1
		}
		cx := st.cursorCol
		if cx >= maxX {
			cx = maxX - 1
		}
		if cx < 0 {
			cx = 0
		}
		scr.Move(cy, cx)
	}
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
	historyLines := buildHistoryLines(thread, maxX)
	historyOffset := 0
	input := &inputState{}
	input.reset()
	focus := focusInput
	needRedraw := true

	for {
		if needRedraw {
			scr.Erase()
			drawThreadHeader(scr, thread)
			drawThreadHistory(scr, historyLines, historyOffset)
			drawThreadInput(scr, input, focus)
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
				continue
			}
		}

		// Compute history view height for scrolling calculations.
		startY := menuHeaderHeight
		endY := maxY - menuStatusHeight - 3
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
				if input.cursorLine > input.scroll+1 {
					input.scroll = input.cursorLine - 1
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
				if input.cursorLine > input.scroll+1 {
					input.scroll = input.cursorLine - 1
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
					wantRetry, modalErr := showErrorRetryModal(scr, err.Error())
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
				// Printable runes are appended to the buffer.
				if ch >= 32 && ch < 127 {
					input.insertRune(rune(ch))
					needRedraw = true
				}
			}
		}
	}
}

func menuMain(ctx context.Context, gptCliCtx *GptCliContext,
	args []string) error {

	if gptCliCtx.mainThreadGroup.totThreads == 0 {
		fmt.Printf("%v.\n", ErrNoThreadsExist)
		return nil
	}

	f := flag.NewFlagSet("ls", flag.ContinueOnError)
	_ = f.Parse(args[1:])

	var sb strings.Builder
	sb.WriteString(gptCliCtx.mainThreadGroup.String(false, false))

	return showMenu(ctx, gptCliCtx, sb.String())
}
