/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
)

type threadMenuEntry struct {
	label      string
	thread     threads.Thread
	isArchived bool
}

type threadMenuUI struct {
	cliCtx   *CliContext
	entries  []threadMenuEntry
	selected int
	offset   int

	// searchQuery enables the "search results" view when non-empty.
	// Search is case-sensitive.
	searchQuery string
}

func newThreadMenuUI(cliCtxIn *CliContext) *threadMenuUI {
	return &threadMenuUI{
		cliCtx:   cliCtxIn,
		entries:  make([]threadMenuEntry, 0),
		selected: 0,
		offset:   0,
		// searchQuery empty => normal list view
		searchQuery: "",
	}
}

func (cliCtx *CliContext) initMenuUI() {
	gc.CBreak(true)
	gc.Echo(false)
	_ = gc.Cursor(0)
	_ = cliCtx.rootWin.Keypad(true)
	cliCtx.rootWin.Timeout(50)

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

			cliCtx.toggles.useColors = true
		}
	}

	cliCtx.menu.resetItems()
}
