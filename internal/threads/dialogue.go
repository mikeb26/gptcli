/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

// ChatOnceInCurrentThread encapsulates the core request/response flow
// for sending a prompt to the current thread, updating dialogue
// history, and persisting the result. It performs no direct terminal
// I/O so callers can render the assistant reply however they choose.
func (thrGrp *GptCliThreadGroup) ChatOnceInCurrentThread(
	ctx context.Context, llmClient types.GptCliAIClient, prompt string,
	summarizePrior bool) (*types.GptCliMessage, error) {

	if thrGrp.curThreadNum == 0 || thrGrp.curThreadNum > thrGrp.totThreads {
		return nil, fmt.Errorf("No thread is currently selected. Select one with 'thread <thread#>'.")
	}

	reqMsg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleUser,
		Content: prompt,
	}

	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	fullDialogue := thread.Dialogue
	summaryDialogue := fullDialogue

	fullDialogue = append(fullDialogue, reqMsg)
	workingDialogue := fullDialogue

	var err error
	if summarizePrior && len(fullDialogue) > 2 {
		summaryDialogue, err = summarizeDialogue(ctx, llmClient, summaryDialogue)
		if err != nil {
			return nil, err
		}
		summaryDialogue = append(summaryDialogue, reqMsg)
		workingDialogue = summaryDialogue
	}

	replyMsg, err := llmClient.CreateChatCompletion(ctx, workingDialogue)
	if err != nil {
		return nil, err
	}

	fullDialogue = append(fullDialogue, replyMsg)
	thread.Dialogue = fullDialogue
	thread.ModTime = time.Now()
	thread.AccessTime = time.Now()

	if err := thread.save(thrGrp.dir); err != nil {
		return nil, err
	}

	return replyMsg, nil
}

// summarizeDialogue summarizes the entire chat history in order to reduce
// llm token costs and refocus the context window
func summarizeDialogue(ctx context.Context, llmClient types.GptCliAIClient,
	dialogue []*types.GptCliMessage) ([]*types.GptCliMessage, error) {

	summaryDialogue := []*types.GptCliMessage{
		{Role: types.GptCliMessageRoleSystem,
			Content: prompts.SystemMsg},
	}

	msg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleSystem,
		Content: prompts.SummarizeMsg,
	}
	dialogue = append(dialogue, msg)

	msg, err := llmClient.CreateChatCompletion(ctx, dialogue)
	if err != nil {
		return summaryDialogue, err
	}

	summaryDialogue = append(summaryDialogue, msg)

	return summaryDialogue, nil
}
