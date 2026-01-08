/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
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

// RenderBlocks flattens the thread dialogue into a sequence of
// RenderBlocks that capture the semantic structure (user prompt,
// assistant text, assistant code) without imposing any particular UI
// representation.
func (t *thread) RenderBlocks() []RenderBlock {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return RenderBlocksFromDialogue(t.persisted.Dialogue)
}

// RenderBlocksFromDialogue flattens a dialogue into a sequence of
// RenderBlocks that capture the semantic structure (user prompt,
// assistant text, assistant code) without imposing any particular UI
// representation.
//
// System messages are omitted.
func RenderBlocksFromDialogue(dialogue []*types.ThreadMessage) []RenderBlock {
	blocks := make([]RenderBlock, 0)

	for _, msg := range dialogue {
		if msg == nil {
			continue
		}
		if msg.Role == types.LlmRoleSystem {
			continue
		}

		switch msg.Role {
		case types.LlmRoleUser:
			blocks = append(blocks, RenderBlock{
				Kind: RenderBlockUserPrompt,
				Text: msg.Content,
			})
		case types.LlmRoleAssistant:
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
