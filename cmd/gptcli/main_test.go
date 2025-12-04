/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestSplitBlocks(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		blocks []string
	}{
		{
			name:   "empty string",
			text:   "",
			blocks: []string{},
		},
		{
			name:   "no code blocks",
			text:   "This is a test.",
			blocks: []string{"This is a test."},
		},
		{
			name:   "single code block",
			text:   "```\ncode block\n```",
			blocks: []string{"", "```\ncode block\n"},
		},
		{
			name:   "text with code blocks",
			text:   "Some text ```\ncode block\n``` follow-up",
			blocks: []string{"Some text ", "```\ncode block\n```", " follow-up"},
		},
		{
			name:   "multiple code blocks",
			text:   "```\nfirst\n``` interlude ```\nsecond\n``` end",
			blocks: []string{"", "```\nfirst\n```", " interlude ", "```\nsecond\n```", " end"},
		},
		{
			name:   "multiline code block",
			text:   "```\nline1\nline2\nline3\n```",
			blocks: []string{"", "```\nline1\nline2\nline3\n"},
		},
		{
			name:   "code block at start and end",
			text:   "```\nstart\n``` text in between ```\nend\n```",
			blocks: []string{"", "```\nstart\n```", " text in between ", "```\nend\n"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitBlocks(tt.text)
			assert.Equal(t, tt.blocks, result)
		})
	}
}

func TestSummarizeDialogue(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := types.NewMockGptCliAIClient(ctrl)
	gptCliCtx := GptCliContext{
		client: mockClient,
	}

	initialDialogue := []*types.GptCliMessage{
		{Role: types.GptCliMessageRoleUser, Content: "Hello!"},
		{Role: types.GptCliMessageRoleAssistant, Content: "Hi! How can I assist you today?"},
	}

	expectedSummaryContent := "User greeted and asked for assistance."
	expectedSummaryMessage := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleAssistant,
		Content: expectedSummaryContent,
	}

	initialDialogueWithSummary := append(initialDialogue, &types.GptCliMessage{
		Role:    types.GptCliMessageRoleSystem,
		Content: prompts.SummarizeMsg,
	})

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Eq(initialDialogueWithSummary)).
		Return(expectedSummaryMessage, nil).Times(1)

	summaryDialogue, err := summarizeDialogue(ctx, &gptCliCtx, initialDialogue)

	assert.NoError(t, err)
	assert.Len(t, summaryDialogue, 2)
	assert.Equal(t, expectedSummaryContent, summaryDialogue[1].Content)
}
