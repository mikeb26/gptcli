/* Copyright © 2025-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"testing"

	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestRenderBlocksSkipsSystemAndSplitsAssistantCode(t *testing.T) {
	thr := &thread{persisted: persistedThread{Dialogue: []*types.ThreadMessage{
		{Role: types.LlmRoleSystem, Content: "sys"},
		{Role: types.LlmRoleUser, Content: "user prompt"},
		{Role: types.LlmRoleAssistant, Content: "before```\ncode\n```after"},
	}}}

	blocks := thr.RenderBlocks()

	// Expect: user prompt, assistant text before, assistant code, assistant text after
	if assert.Len(t, blocks, 4) {
		assert.Equal(t, RenderBlockUserPrompt, blocks[0].Kind)
		assert.Equal(t, "user prompt", blocks[0].Text)

		assert.Equal(t, RenderBlockAssistantText, blocks[1].Kind)
		assert.Equal(t, "before", blocks[1].Text)

		assert.Equal(t, RenderBlockAssistantCode, blocks[2].Kind)
		assert.Contains(t, blocks[2].Text, "code")

		assert.Equal(t, RenderBlockAssistantText, blocks[3].Kind)
		assert.Equal(t, "after", blocks[3].Text)
	}
}
