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
)

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
			useColors = true
		}
	}

	ui := newThreadMenuUI(scr, useColors)
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
			if len(ui.items) == 0 {
				return nil
			}
			threadIndex := ui.selected + 1 // threads are 1-based
			gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
			// For now we still delegate to the existing CLI-thread view by
			// calling threadSwitch, which prints the thread contents and
			// exits the menu. In a future change, this will transition into
			// an ncurses-based thread view (runThreadView) instead of
			// dropping back to the basic prompt.
			return gptCliCtx.mainThreadGroup.threadSwitch(threadIndex)
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
