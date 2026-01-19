/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	"context"
	"fmt"

	gc "github.com/rthornton128/goncurses"
)

// drawThreadHeader renders a single-line header for the thread view.
func (tvUI *threadViewUI) drawThreadHeader(ctx context.Context) {
	_, maxX := tvUI.cliCtx.rootWin.MaxYX()
	header := fmt.Sprintf("Thread: %s", tvUI.thread.Name())
	repoStatus := "<none>"
	if tvUI.workDir != "" {
		repoStatus2, err := tvUI.cliCtx.scmClient.RepoStatusString(ctx, tvUI.workDir)
		if err == nil {
			repoStatus = repoStatus2
		}
	}
	header = fmt.Sprintf("%s | Repo: %s", header, repoStatus)

	if len([]rune(header)) > maxX {
		header = string([]rune(header)[:maxX])
	}

	var attr gc.Char = gc.A_NORMAL
	if tvUI.cliCtx.toggles.useColors {
		attr |= gc.ColorPair(menuColorHeader)
	}
	_ = tvUI.cliCtx.rootWin.AttrSet(attr)
	for x := 0; x < maxX; x++ {
		tvUI.cliCtx.rootWin.MoveAddChar(0, x, gc.Char(' ')|attr)
	}
	_ = tvUI.cliCtx.rootWin.TouchLine(0, 1)
	tvUI.cliCtx.rootWin.MovePrint(0, 0, header)
	_ = tvUI.cliCtx.rootWin.AttrSet(gc.A_NORMAL)
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
	if focus == focusHistory {
		segments = append(segments, []statusSegment{
			{text: " Commit:", bold: false},
			{text: "c", bold: true},
			{text: " Diff:", bold: false},
			{text: "d", bold: true},
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
