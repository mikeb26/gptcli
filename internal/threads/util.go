/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
)

const (
	CodeBlockDelim        = "```"
	CodeBlockDelimNewline = "```\n"
)

// RenderBlockKind identifies the semantic type of a block of text in a
// thread dialogue. This is UI-agnostic so that different frontends
// (classic CLI, ncurses, etc.) can render the same logical content with
// their own styling.
type RenderBlockKind int

const (
	RenderBlockUserPrompt RenderBlockKind = iota
	RenderBlockAssistantText
	RenderBlockAssistantCode
)

// RenderBlock represents a contiguous span of text with a particular
// semantic role. It does not contain any ANSI color or formatting
// information; callers are expected to style it appropriately.
type RenderBlock struct {
	Kind RenderBlockKind
	Text string
}

// formatHeaderTime renders a timestamp for use in the thread list header.
// If the time falls on the same local calendar day as "now", the date
// portion is replaced with "Today". If it falls on the preceding
// calendar day, it is replaced with "Yesterday". Otherwise, the full
// date is shown. Calendar-day comparisons are done in the local time
// zone associated with "now" to avoid off-by-one errors around
// midnight or when using non-UTC locations.
func formatHeaderTime(ts time.Time, now time.Time) string {
	// Normalize the target time into the same location as "now" so
	// that calendar-day comparisons are meaningful.
	ts = ts.In(now.Location())

	full := ts.Format("01/02/2006 03:04pm")
	datePart := ts.Format("01/02/2006")

	y, m, d := now.Date()
	todayY, todayM, todayD := y, m, d
	yest := now.AddDate(0, 0, -1)
	yestY, yestM, yestD := yest.Date()
	ty, tm, td := ts.Date()

	switch {
	case ty == todayY && tm == todayM && td == todayD:
		return strings.Replace(full, datePart, "Today", 1)
	case ty == yestY && tm == yestM && td == yestD:
		return strings.Replace(full, datePart, "Yesterday", 1)
	default:
		return full
	}
}

// RenderBlocks flattens the thread dialogue into a sequence of
// RenderBlocks that capture the semantic structure (user prompt,
// assistant text, assistant code) without imposing any particular UI
// representation.
func (thread *Thread) RenderBlocks() []RenderBlock {
	thread.mu.RLock()
	defer thread.mu.RUnlock()

	blocks := make([]RenderBlock, 0)

	for _, msg := range thread.persisted.Dialogue {
		if msg.Role == types.GptCliMessageRoleSystem {
			continue
		}

		switch msg.Role {
		case types.GptCliMessageRoleUser:
			blocks = append(blocks, RenderBlock{
				Kind: RenderBlockUserPrompt,
				Text: msg.Content,
			})
		case types.GptCliMessageRoleAssistant:
			parts := splitBlocks(msg.Content)
			for idx, p := range parts {
				kind := RenderBlockAssistantText
				if idx%2 == 1 {
					kind = RenderBlockAssistantCode
				}
				blocks = append(blocks, RenderBlock{
					Kind: kind,
					Text: p,
				})
			}
		}
	}

	return blocks
}

func genUniqFileName(name string, cTime time.Time) string {
	return fmt.Sprintf("%v_%v.json",
		strconv.FormatUint(uint64(crc32.ChecksumIEEE([]byte(name))), 16),
		cTime.Unix())
}

func splitBlocks(text string) []string {
	blocks := make([]string, 0)

	inBlock := false
	idx := strings.Index(text, CodeBlockDelim)
	numBlocks := 0
	for ; idx != -1; idx = strings.Index(text, CodeBlockDelim) {
		appendText := text[0:idx]
		if inBlock {
			appendText = CodeBlockDelim + appendText
		} else if numBlocks != 0 {
			blocks[numBlocks-1] = blocks[numBlocks-1] + CodeBlockDelim
		}
		blocks = append(blocks, appendText)
		text = text[idx+len(CodeBlockDelim):]
		inBlock = !inBlock
		numBlocks++
	}
	if len(text) > 0 {
		if inBlock {
			text = text + CodeBlockDelim
		} else if numBlocks != 0 {
			blocks[numBlocks-1] = blocks[numBlocks-1] + CodeBlockDelim
		}
		blocks = append(blocks, text)
	}

	return blocks
}
