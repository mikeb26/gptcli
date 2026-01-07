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
	"time"

	"github.com/famz/SetLocale"
	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
	iui "github.com/mikeb26/gptcli/internal/ui"
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
	iui.AdjustListViewport(total, vh, &ui.selected, &ui.offset)
}

func (ui *threadMenuUI) draw() {
	scr := ui.scr

	maxY, maxX := scr.MaxYX()
	vh := ui.viewHeight()

	ui.adjustOffset()

	headerTitle := strings.Split(threads.ThreadGroupHeaderString(false), "\n")[0]
	headerTitle = iui.TruncateRunes(headerTitle, maxX)

	if ui.useColors {
		_ = scr.AttrSet(gc.A_NORMAL | gc.ColorPair(menuColorHeader))
	} else {
		_ = scr.AttrSet(gc.A_NORMAL)
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
			line := iui.TruncateRunes(ui.items[idx], maxX)

			if idx == ui.selected {
				if ui.useColors {
					_ = scr.AttrSet(gc.A_NORMAL | gc.ColorPair(menuColorSelected))
				} else {
					_ = scr.AttrSet(gc.A_REVERSE | gc.A_NORMAL)
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
		segments := []statusSegment{
			{text: "Nav:", bold: false},
			{text: "↑", bold: true},
			{text: "/", bold: false},
			{text: "↓", bold: true},
			{text: "/", bold: false},
			{text: "PgUp", bold: true},
			{text: "/", bold: false},
			{text: "PgDn", bold: true},
			{text: "/", bold: false},
			{text: "Home", bold: true},
			{text: "/", bold: false},
			{text: "End", bold: true},
			{text: " Select:", bold: false},
			{text: "⏎", bold: true},
			{text: " New:", bold: false},
			{text: "n", bold: true},
			{text: " Archive:", bold: false},
			{text: "a", bold: true},
			{text: " Config:", bold: false},
			{text: "c", bold: true},
			{text: " Quit:", bold: false},
			{text: "ESC", bold: true},
		}
		drawStatusSegments(scr, statusY, maxX, segments, ui.useColors)
	}

	_ = scr.AttrSet(gc.A_NORMAL)
	scr.Refresh()
}

func (ui *threadMenuUI) resetItems(menuText string) error {
	trimmed := strings.TrimRight(menuText, "\n")
	if trimmed == "" {
		return fmt.Errorf("empty menu text")
	}

	ui.items = strings.Split(trimmed, "\n")

	return nil
}

func gcInit() (*gc.Window, error) {
	// Require a real TTY; ncurses UI is not supported otherwise
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, fmt.Errorf("menu: requires a terminal (TTY)")
	}

	SetLocale.SetLocale(SetLocale.LC_ALL, "en_US.UTF-8")
	// Reduce ncurses' ESC-key delay so pressing ESC is responsive.
	//
	// In keypad mode, ncurses must disambiguate a literal ESC press from
	// an escape sequence (e.g. arrow keys), and it does so by waiting up
	// to ESCDELAY milliseconds for additional bytes.
	//
	// This MUST be set before initializing ncurses via gc.Init().
	_ = os.Setenv("ESCDELAY", "100")
	// Ensure environment is consistent for UTF-8 rendering.
	_ = os.Setenv("LANG", "en_US.UTF-8")
	_ = os.Setenv("LC_ALL", "en_US.UTF-8")
	scr, err := gc.Init()
	if err != nil {
		return nil, fmt.Errorf("Failed to initialize screen: %w", err)
	}

	return scr, nil
}

func gcExit() {
	gc.End()
}

func showMenu(ctx context.Context, gptCliCtx *CliContext, menuText string) error {

	//scr, err := gcInit()
	//if err != nil {
	//return err
	//}
	//defer gcExit()
	scr := gptCliCtx.scr
	if scr == nil {
		panic("nil scr")
	}

	// Listen for SIGWINCH (terminal resize). We handle the signal in this
	// same goroutine by polling the channel inside the UI loop, which
	// keeps all ncurses interaction single-threaded.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	menuUI, err := initUI(scr, menuText)
	if err != nil {
		return err
	}
	needErase := true
	needRefresh := false
	upgradeChecked := false
	ncui := gptCliCtx.realUI

	// Keep internal/ui modal selection styling consistent with the menu's
	// colors (or fall back to reverse-video in monochrome mode).
	ncui.SetTheme(iui.Theme{UseColors: menuUI.useColors, SelectedPair: menuColorSelected})
	lastRefresh := time.Now()

	for {
		if needErase {
			scr.Erase()
			needErase = false
		}
		if needRefresh {
			if err := menuUI.resetItems(gptCliCtx.mainThreadGroup.String(false, false)); err != nil {
				return err
			}
			if menuUI.selected >= len(menuUI.items) {
				menuUI.selected = len(menuUI.items) - 1
			}
			needRefresh = false
			lastRefresh = time.Now()
		}

		menuUI.draw()
		if !upgradeChecked {
			upgradeIfNeeded(ctx, gptCliCtx)
			upgradeChecked = true
		}

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(scr)
			needErase = true
			continue
		default:
			ch = scr.GetChar()
			if ch == 0 {
				if time.Now().Sub(lastRefresh) > 200*time.Millisecond {
					needRefresh = true
				}
				continue
			}
		}

		switch ch {
		case gc.Key(27): // ESC
			return nil
		case gc.KEY_UP:
			if menuUI.selected > 0 {
				menuUI.selected--
			}
		case gc.KEY_DOWN:
			if menuUI.selected < len(menuUI.items)-1 {
				menuUI.selected++
			}
		case gc.KEY_HOME:
			menuUI.selected = 0
		case gc.KEY_END:
			if len(menuUI.items) > 0 {
				menuUI.selected = len(menuUI.items) - 1
			}
		case gc.KEY_PAGEUP:
			if vh := menuUI.viewHeight(); vh > 0 {
				menuUI.selected -= vh
				if menuUI.selected < 0 {
					menuUI.selected = 0
				}
			}
		case gc.KEY_PAGEDOWN:
			if vh := menuUI.viewHeight(); vh > 0 {
				menuUI.selected += vh
				if menuUI.selected > len(menuUI.items)-1 {
					menuUI.selected = len(menuUI.items) - 1
				}
			}
		case gc.KEY_ENTER, gc.KEY_RETURN:
			// Enter: activate the selected thread and transition into the
			// ncurses-based thread view instead of dropping back to the
			// basic CLI prompt.
			if len(menuUI.items) == 0 {
				continue
			}
			threadIndex := menuUI.selected + 1 // threads are 1-based
			gptCliCtx.curThreadGroup = gptCliCtx.mainThreadGroup
			thread, err := gptCliCtx.mainThreadGroup.ActivateThread(threadIndex)
			if err != nil {
				// Propagate the error so the caller can handle it and
				// exit ncurses cleanly.
				return err
			}
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
			name, err := promptForThreadNameNCurses(ncui)
			if err != nil {
				return fmt.Errorf("gptcli: failed to prompt for new thread name: %w", err)
			}
			if name == "" { // user cancelled
				continue
			}
			if err := gptCliCtx.mainThreadGroup.NewThread(name); err != nil {
				return fmt.Errorf("gptcli: failed to create new thread from menu: %w", err)
			}
			needRefresh = true
		case 'c':
			configMain(ctx, gptCliCtx)
		case 'a':
			needErase = true
			// Archive the currently selected thread from the main thread group.
			// This mirrors the behavior of archiveThreadMain(), but uses the
			// selection from the menu UI instead of parsing a CLI argument.
			if len(menuUI.items) == 0 {
				continue
			}
			threadIndex := menuUI.selected + 1 // threads are 1-based

			// Only main-thread-group entries are shown in the menu, so we move
			// from mainThreadGroup to archiveThreadGroup directly.
			if gptCliCtx.mainThreadGroup.Count() == 0 {
				continue
			}
			if threadIndex > gptCliCtx.mainThreadGroup.Count() {
				continue
			}

			threadId := gptCliCtx.mainThreadGroup.ThreadId(threadIndex)
			// @todo should cleanup thread.{asyncApprover, llmClient}
			if err := gptCliCtx.mainThreadGroup.MoveThread(threadIndex, gptCliCtx.archiveThreadGroup); err != nil {
				return fmt.Errorf("gptcli: failed to archive thread from menu: %w", err)
			}

			delete(gptCliCtx.asyncChatUIStates, threadId)
			needRefresh = true
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
