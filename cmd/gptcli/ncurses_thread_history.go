/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package main

import (
	gc "github.com/gbin/goncurses"
	"github.com/mikeb26/gptcli/internal/threads"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/mikeb26/gptcli/internal/ui"
)

// buildHistoryLines converts the logical RenderBlocks for a thread
// into a flat slice of ui.FrameLine values, applying prefixes ("You:",
// "LLM:") and soft wrapping with a trailing '\\' on wrapped
// segments. The resulting slice is suitable for direct line-by-line
// rendering in the history pane via a ui.Frame.
func buildHistoryLines(cliCtx *CliContext, blocks []threads.RenderBlock,
	width int) []ui.FrameLine {

	// We need at least two columns: one for text and one for the history
	// frame's scrollbar. Below that threshold we simply omit history
	// rendering.
	if width <= 1 {
		return nil
	}
	lines := make([]ui.FrameLine, 0)

	// The history frame reserves its last column for a scrollbar, so we
	// only have (width-1) columns available for text. Wrapping must obey
	// this limit or Frame.Render will apply an additional truncation
	// (including its own '\\' marker) and drop characters at wrap
	// boundaries.
	textWidth := width - 1
	if textWidth < 1 {
		textWidth = 1
	}
	for _, b := range blocks {
		var prefix string
		attr := gc.A_NORMAL

		switch b.Kind {
		case threads.RenderBlockUserPrompt:
			prefix = "You: "
			if cliCtx.toggles.useColors {
				attr = gc.ColorPair(threadColorUser)
			} else {
				attr = gc.A_BOLD
			}
		case threads.RenderBlockAssistantText:
			prefix = "LLM: "
			if cliCtx.toggles.useColors {
				attr = gc.ColorPair(threadColorAssistant)
			} else {
				attr = gc.A_NORMAL
			}
		case threads.RenderBlockAssistantCode:
			prefix = "LLM: "
			if cliCtx.toggles.useColors {
				attr = gc.ColorPair(threadColorCode)
			} else {
				attr = gc.A_BOLD
			}
		}

		lines = append(lines, ui.WrapTextWithPrefix(prefix, b.Text, textWidth, attr)...)
	}

	return lines
}

func buildHistoryLinesForThread(cliCtx *CliContext, thread threads.Thread,
	width int) []ui.FrameLine {

	return buildHistoryLines(cliCtx, thread.RenderBlocks(), width)
}

func (tvUI *threadViewUI) setHistoryFrameFromBlocks(
	blocks []threads.RenderBlock,
	extraAssistantText string,
) {
	fullBlocks := append([]threads.RenderBlock(nil), blocks...)
	if extraAssistantText != "" {
		extraBlocks := threads.RenderBlocksFromDialogue([]*types.ThreadMessage{{
			Role:    types.LlmRoleAssistant,
			Content: extraAssistantText,
		}})
		fullBlocks = append(fullBlocks, extraBlocks...)
	}
	_, maxX := tvUI.cliCtx.rootWin.MaxYX()
	lines := buildHistoryLines(tvUI.cliCtx, fullBlocks, maxX)
	tvUI.historyFrame.SetLines(lines)
	tvUI.historyFrame.MoveEnd()
}

func (tvUI *threadViewUI) setHistoryFrameForThread() {
	_, maxX := tvUI.cliCtx.rootWin.MaxYX()
	tvUI.historyFrame.SetLines(buildHistoryLinesForThread(tvUI.cliCtx, tvUI.thread, maxX))
	tvUI.historyFrame.MoveEnd()
}

func threadViewDisplayBlocks(thread threads.Thread, pendingPrompt string) []threads.RenderBlock {
	blocks := append([]threads.RenderBlock(nil), thread.RenderBlocks()...)
	if pendingPrompt != "" {
		blocks = append(blocks, threads.RenderBlock{Kind: threads.RenderBlockUserPrompt, Text: pendingPrompt})
	}
	return blocks
}
