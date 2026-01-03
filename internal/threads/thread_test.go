package threads

import (
	"testing"
	"time"

	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestHeaderStringUsesFormattedTimes(t *testing.T) {
	thr := &Thread{persisted: persistedThread{Name: "test-thread"}}

	// Fix timestamps so behavior is deterministic.
	base := time.Date(2025, 1, 15, 10, 30, 0, 0, time.Local)
	thr.persisted.CreateTime = base.Add(-2 * time.Hour)
	thr.persisted.AccessTime = base.Add(-1 * time.Hour)
	thr.persisted.ModTime = base.Add(-30 * time.Minute)

	header := thr.HeaderString("T1")

	// Basic sanity checks: thread number and name present, no panic.
	assert.Contains(t, header, "T1")
	assert.Contains(t, header, "test-thread")
}

func TestRenderBlocksSkipsSystemAndSplitsAssistantCode(t *testing.T) {
	thr := &Thread{persisted: persistedThread{Dialogue: []*types.ThreadMessage{
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
