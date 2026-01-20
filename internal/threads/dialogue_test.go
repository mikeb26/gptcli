/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
	"github.com/stretchr/testify/assert"
)

func TestSummarizeDialogue(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClient := types.NewMockAIClient(ctrl)

	sysMsg := &types.ThreadMessage{Role: types.LlmRoleSystem, Content: "system"}

	initialDialogue := []*types.ThreadMessage{
		{Role: types.LlmRoleUser, Content: "Hello!"},
		{Role: types.LlmRoleAssistant, Content: "Hi! How can I assist you today?"},
	}

	expectedSummaryContent := "User greeted and asked for assistance."
	expectedSummaryMessage := &types.ThreadMessage{
		Role:    types.LlmRoleAssistant,
		Content: expectedSummaryContent,
	}

	initialDialogueWithSummary := append(initialDialogue, &types.ThreadMessage{
		Role:    types.LlmRoleSystem,
		Content: prompts.SummarizeMsg,
	})

	mockClient.EXPECT().
		CreateChatCompletion(gomock.Any(), gomock.Eq(initialDialogueWithSummary)).
		Return(expectedSummaryMessage, nil).Times(1)

	summaryDialogue, err := summarizeDialogue(ctx, mockClient, sysMsg, initialDialogue)

	assert.NoError(t, err)
	assert.Len(t, summaryDialogue, 2)
	assert.Equal(t, sysMsg, summaryDialogue[0])
	assert.Equal(t, expectedSummaryContent, summaryDialogue[1].Content)
}
