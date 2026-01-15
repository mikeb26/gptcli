/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"fmt"

	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
)

// drawThreadHeader renders a single-line header for the thread view.
func drawThreadHeader(cliCtx *CliContext, thread threads.Thread) {
	_, maxX := cliCtx.rootWin.MaxYX()
	header := fmt.Sprintf("Thread: %s", thread.Name())
	if len([]rune(header)) > maxX {
		header = string([]rune(header)[:maxX])
	}

	var attr gc.Char = gc.A_NORMAL
	if cliCtx.toggles.useColors {
		attr |= gc.ColorPair(menuColorHeader)
	}
	_ = cliCtx.rootWin.AttrSet(attr)
	for x := 0; x < maxX; x++ {
		cliCtx.rootWin.MoveAddChar(0, x, gc.Char(' ')|attr)
	}
	_ = cliCtx.rootWin.TouchLine(0, 1)
	cliCtx.rootWin.MovePrint(0, 0, header)
	_ = cliCtx.rootWin.AttrSet(gc.A_NORMAL)
}

// drawNavbar renders a simple status line at the bottom of the
// screen, including mode information and key hints.
func drawNavbar(cliCtx *CliContext, focus threadViewFocus, isArchived bool) {
	maxY, maxX := cliCtx.rootWin.MaxYX()
	statusY := maxY - 1
	if statusY < 0 {
		return
	}

	segments := []statusSegment{
		{text: "Nav:", bold: false},
		{text: "↑", bold: true},
		{text: "/", bold: false},
		{text: "↓", bold: true},
		{text: "/", bold: false},
		{text: "→", bold: true},
		{text: "/", bold: false},
		{text: "←", bold: true},
		{text: "/", bold: false},
		{text: "PgUp", bold: true},
		{text: "/", bold: false},
		{text: "PgDn", bold: true},
		{text: "/", bold: false},
		{text: "Home", bold: true},
		{text: "/", bold: false},
		{text: "End", bold: true},
	}
	if !isArchived {
		segments = append(segments, []statusSegment{
			{text: " OtherWin:", bold: false},
			{text: "Tab", bold: true},
			{text: " Send:", bold: false},
			{text: "Ctrl-d", bold: true},
		}...)
	}
	segments = append(segments, []statusSegment{
		{text: " Back:", bold: false},
		{text: "ESC", bold: true},
	}...)
	drawStatusSegments(cliCtx.rootWin, statusY, maxX, segments,
		cliCtx.toggles.useColors)

}

func (tvUI *threadViewUI) getFocus() threadViewFocus {
	if tvUI.focusedFrame == tvUI.historyFrame {
		return focusHistory
	}
	return focusInput
}
