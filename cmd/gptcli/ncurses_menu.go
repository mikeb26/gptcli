/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
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
	"github.com/mikeb26/gptcli/internal/types"
	iui "github.com/mikeb26/gptcli/internal/ui"
	"golang.org/x/term"
)

func confirmQuitIfNonIdleThreads(gptCliCtx *CliContext) (bool, error) {
	if gptCliCtx == nil {
		return true, nil
	}

	nonIdle := 0
	if gptCliCtx.mainThreadGroup != nil {
		nonIdle += gptCliCtx.mainThreadGroup.NonIdleThreadCount()
	}
	if gptCliCtx.archiveThreadGroup != nil {
		nonIdle += gptCliCtx.archiveThreadGroup.NonIdleThreadCount()
	}

	if nonIdle == 0 {
		return true, nil
	}

	prompt := fmt.Sprintf(
		"You have %d non-idle thread(s) (running or awaiting approval). Quit anyway?\n\nIf you quit now, you may lose progress/output.",
		nonIdle,
	)
	defaultQuit := false
	trueOpt := types.UIOption{Key: "y", Label: "y"}
	falseOpt := types.UIOption{Key: "n", Label: "n"}
	quit, err := gptCliCtx.ui.SelectBool(prompt, trueOpt, falseOpt, &defaultQuit)
	if err != nil {
		return false, err
	}

	return quit, nil
}

const (
	menuHeaderHeight         = 1
	menuStatusHeight         = 1
	menuColorHeader    int16 = 1
	menuColorStatus    int16 = 2
	menuColorSelected  int16 = 3
	menuColorStatusKey int16 = 4
)

func (ui *threadMenuUI) viewHeight() int {
	maxY, _ := ui.cliCtx.rootWin.MaxYX()
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
	scr := ui.cliCtx.rootWin

	maxY, maxX := scr.MaxYX()
	vh := ui.viewHeight()

	ui.adjustOffset()

	headerTitle := strings.Split(threadGroupHeaderString(false), "\n")[0]
	headerTitle = iui.TruncateRunes(headerTitle, maxX)

	if ui.cliCtx.toggles.useColors {
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
				if ui.cliCtx.toggles.useColors {
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
		drawStatusSegments(scr, statusY, maxX, segments, ui.cliCtx.toggles.useColors)
	}

	_ = scr.AttrSet(gc.A_NORMAL)
	scr.Refresh()
}

func (ui *threadMenuUI) resetItems(menuText string) error {
	trimmed := strings.TrimRight(menuText, "\n")
	if trimmed == "" {
		return ErrEmptyMenuText
	}

	ui.items = strings.Split(trimmed, "\n")

	return nil
}

func gcInit() (*gc.Window, error) {
	// Require a real TTY; ncurses UI is not supported otherwise
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return nil, ErrTTYRequired
	}

	// Reduce ncurses' ESC-key delay so pressing ESC is responsive.
	//
	// In keypad mode, ncurses must disambiguate a literal ESC press from
	// an escape sequence (e.g. arrow keys), and it does so by waiting up
	// to ESCDELAY milliseconds for additional bytes.
	//
	// This MUST be set before initializing ncurses via gc.Init().
	_ = os.Setenv("ESCDELAY", "100")
	// Enable UTF-8; must similarly be set before gc.Init()
	SetLocale.SetLocale(SetLocale.LC_ALL, "en_US.UTF-8")
	rootWin, err := gc.Init()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToInitScreen, err)
	}

	return rootWin, nil
}

func gcExit() {
	gc.End()
}

func showMenu(ctx context.Context, cliCtx *CliContext, menuText string) error {
	// Listen for SIGWINCH (terminal resize). We handle the signal in this
	// same goroutine by polling the channel inside the UI loop, which
	// keeps all ncurses interaction single-threaded.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	cliCtx.initMenuUI(menuText)
	needErase := true
	needRefresh := false
	upgradeChecked := false

	// Keep internal/ui modal selection styling consistent with the menu's
	// colors (or fall back to reverse-video in monochrome mode).
	cliCtx.ui.SetTheme(iui.Theme{UseColors: cliCtx.toggles.useColors, SelectedPair: menuColorSelected})
	lastRefresh := time.Now()

	for {
		if needErase {
			cliCtx.rootWin.Erase()
			needErase = false
		}
		if needRefresh {
			if err :=
				cliCtx.menu.resetItems(threadGroupString(cliCtx.mainThreadGroup, false, false)); err != nil {
				return err
			}
			if cliCtx.menu.selected >= len(cliCtx.menu.items) {
				cliCtx.menu.selected = len(cliCtx.menu.items) - 1
			}
			needRefresh = false
			lastRefresh = time.Now()
		}

		cliCtx.menu.draw()
		if !upgradeChecked {
			upgradeIfNeeded(ctx, cliCtx)
			upgradeChecked = true
		}

		var ch gc.Key
		select {
		case <-sigCh:
			resizeScreen(cliCtx.rootWin)
			needErase = true
			continue
		default:
			ch = cliCtx.rootWin.GetChar()
			if ch == 0 {
				if time.Now().Sub(lastRefresh) > 200*time.Millisecond {
					needRefresh = true
				}
				continue
			}
		}

		switch ch {
		case gc.Key(27): // ESC
			quit, err := confirmQuitIfNonIdleThreads(cliCtx)
			if err != nil {
				return err
			}
			if quit {
				return nil
			}
			needErase = true
			continue
		case gc.KEY_UP:
			if cliCtx.menu.selected > 0 {
				cliCtx.menu.selected--
			}
		case gc.KEY_DOWN:
			if cliCtx.menu.selected < len(cliCtx.menu.items)-1 {
				cliCtx.menu.selected++
			}
		case gc.KEY_HOME:
			cliCtx.menu.selected = 0
		case gc.KEY_END:
			if len(cliCtx.menu.items) > 0 {
				cliCtx.menu.selected = len(cliCtx.menu.items) - 1
			}
		case gc.KEY_PAGEUP:
			if vh := cliCtx.menu.viewHeight(); vh > 0 {
				cliCtx.menu.selected -= vh
				if cliCtx.menu.selected < 0 {
					cliCtx.menu.selected = 0
				}
			}
		case gc.KEY_PAGEDOWN:
			if vh := cliCtx.menu.viewHeight(); vh > 0 {
				cliCtx.menu.selected += vh
				if cliCtx.menu.selected > len(cliCtx.menu.items)-1 {
					cliCtx.menu.selected = len(cliCtx.menu.items) - 1
				}
			}
		case gc.KEY_ENTER, gc.KEY_RETURN:
			// Enter: activate the selected thread and transition into the
			// ncurses-based thread view instead of dropping back to the
			// basic CLI prompt.
			if len(cliCtx.menu.items) == 0 {
				continue
			}
			threadIndex := cliCtx.menu.selected + 1 // threads are 1-based
			cliCtx.curThreadGroup = cliCtx.mainThreadGroup
			thread, err := cliCtx.mainThreadGroup.ActivateThread(threadIndex)
			if err != nil {
				// Propagate the error so the caller can handle it and
				// exit ncurses cleanly.
				return err
			}
			if err := runThreadView(ctx, cliCtx, thread); err != nil {
				return err
			}
			// After returning from the thread view, redraw the menu.
			needErase = true
		case 'n', 'N':
			needErase = true
			// Create a new thread (equivalent to the "new" subcommand), but
			// prompt for the name using an ncurses modal so we don't mix
			// stdio with the UI.
			name, err := promptForThreadNameNCurses(cliCtx.ui)
			if err != nil {
				return fmt.Errorf("%w: %w", ErrFailedToPromptThreadName, err)
			}
			if name == "" { // user cancelled
				continue
			}
			if err := cliCtx.mainThreadGroup.NewThread(name); err != nil {
				return fmt.Errorf("%w: %w", ErrFailedToCreateThread, err)
			}
			needRefresh = true
		case 'c':
			configMain(ctx, cliCtx)
		case 'a':
			needErase = true
			// Archive the currently selected thread from the main thread group.
			// This mirrors the behavior of archiveThreadMain(), but uses the
			// selection from the menu UI instead of parsing a CLI argument.
			if len(cliCtx.menu.items) == 0 {
				continue
			}
			threadIndex := cliCtx.menu.selected + 1 // threads are 1-based

			// Only main-thread-group entries are shown in the menu, so we move
			// from mainThreadGroup to archiveThreadGroup directly.
			if cliCtx.mainThreadGroup.Count() == 0 {
				continue
			}
			if threadIndex > cliCtx.mainThreadGroup.Count() {
				continue
			}

			threadId := cliCtx.mainThreadGroup.ThreadId(threadIndex)
			// @todo should cleanup thread.{asyncApprover, llmClient}
			if err := cliCtx.mainThreadGroup.MoveThread(threadIndex, cliCtx.archiveThreadGroup); err != nil {
				return fmt.Errorf("%w: %w", ErrFailedToArchiveThread, err)
			}

			delete(cliCtx.asyncChatUIStates, threadId)
			needRefresh = true
		case gc.KEY_RESIZE:
			resizeScreen(cliCtx.rootWin)
			needErase = true
			continue
		}
	}
}
