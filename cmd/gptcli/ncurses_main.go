/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"

	gc "github.com/gbin/goncurses"
)

type threadMenuUI struct {
	scr       *gc.Window
	items     []string
	selected  int
	offset    int
	useColors bool
}

// globalUseColors mirrors the color capability detected in initUI so
// that other ncurses views (like the per-thread view) can make the
// same color vs monochrome decisions without re-detecting.
var globalUseColors bool

func newThreadMenuUI(scr *gc.Window, useColors bool) *threadMenuUI {
	return &threadMenuUI{
		scr:       scr,
		items:     make([]string, 0),
		selected:  0,
		offset:    0,
		useColors: useColors,
	}
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
