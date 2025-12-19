/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/mikeb26/gptcli/internal/llmclient"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

// PreparedChat captures the state needed to complete a single request
// within the current thread. It is UI-agnostic so that both streaming
// and non-streaming callers can share the same preparation and
// finalization logic.
type PreparedChat struct {
	Ctx             context.Context
	Thread          *GptCliThread
	FullDialogue    []*types.GptCliMessage // full history + user request
	WorkingDialogue []*types.GptCliMessage // possibly summarized + user request
	ReqMsg          *types.GptCliMessage
	InvocationID    string
}

// prepareChatOnceInCurrentThread performs all work needed before
// sending a request to the LLM: validating the current thread,
// constructing the user message, optionally summarizing prior dialogue,
// and returning both the full and working dialogue slices.
func (thrGrp *GptCliThreadGroup) prepareChatOnceInCurrentThread(
	ctx context.Context, llmClient types.GptCliAIClient, prompt string,
	summarizePrior bool,
) (*PreparedChat, error) {

	if thrGrp.curThreadNum == 0 || thrGrp.curThreadNum > thrGrp.totThreads {
		return nil, fmt.Errorf("No thread is currently selected. Select one with 'thread <thread#>'.")
	}

	reqMsg := &types.GptCliMessage{
		Role:    types.GptCliMessageRoleUser,
		Content: prompt,
	}

	thread := thrGrp.threads[thrGrp.curThreadNum-1]
	thread.state = GptCliThreadStateRunning
	// Attach a thread-state setter so lower layers (e.g. tool approval prompts)
	// can signal when the active thread is blocked waiting on user interaction
	// without creating an import cycle.
	ctx = types.WithThreadStateSetter(ctx, &threadStateSetter{thread: thread})
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

	prep := &PreparedChat{
		Ctx:             ctx,
		Thread:          thread,
		FullDialogue:    fullDialogue,
		WorkingDialogue: workingDialogue,
		ReqMsg:          reqMsg,
	}

	return prep, nil
}

// FinalizeChatOnceInCurrentThread appends the assistant reply to the
// thread's dialogue, updates timestamps, and persists the thread to
// disk.
func (thrGrp *GptCliThreadGroup) FinalizeChatOnceInCurrentThread(
	prep *PreparedChat, replyMsg *types.GptCliMessage,
) error {
	if prep == nil || prep.Thread == nil {
		return fmt.Errorf("invalid prepared chat: missing thread")
	}

	thread := prep.Thread
	fullDialogue := append(prep.FullDialogue, replyMsg)
	thread.Dialogue = fullDialogue
	thread.ModTime = time.Now()
	thread.AccessTime = time.Now()
	thread.state = GptCliThreadStateIdle

	if err := thread.save(thrGrp.dir); err != nil {
		return err
	}

	return nil
}

// ChatOnceInCurrentThread encapsulates the core request/response flow
// for sending a prompt to the current thread using a non-streaming LLM
// call, updating dialogue history, and persisting the result.
func (thrGrp *GptCliThreadGroup) ChatOnceInCurrentThread(
	ctx context.Context, llmClient types.GptCliAIClient, prompt string,
	summarizePrior bool) (*types.GptCliMessage, error) {

	prep, err := thrGrp.prepareChatOnceInCurrentThread(ctx, llmClient, prompt, summarizePrior)
	if err != nil {
		return nil, err
	}

	replyMsg, err := llmClient.CreateChatCompletion(prep.Ctx, prep.WorkingDialogue)
	if err != nil {
		return nil, err
	}

	if err := thrGrp.FinalizeChatOnceInCurrentThread(prep, replyMsg); err != nil {
		return nil, err
	}

	return replyMsg, nil
}

// ChatOnceInCurrentThreadStream prepares the current thread dialogue
// for a new user prompt and returns both the PreparedChat and a
// streaming reader for the assistant reply. Callers are responsible for
// consuming the stream, assembling the final reply message, and then
// invoking FinalizeChatOnceInCurrentThread.
func (thrGrp *GptCliThreadGroup) ChatOnceInCurrentThreadStream(
	ctx context.Context, llmClient types.GptCliAIClient, prompt string,
	summarizePrior bool,
) (*PreparedChat, *schema.StreamReader[*types.GptCliMessage], error) {

	prep, err := thrGrp.prepareChatOnceInCurrentThread(ctx, llmClient, prompt, summarizePrior)
	if err != nil {
		return nil, nil, err
	}

	prep.Ctx, prep.InvocationID = llmclient.EnsureInvocationID(prep.Ctx)
	res, err := llmClient.StreamChatCompletion(prep.Ctx, prep.WorkingDialogue)
	if err != nil {
		return nil, nil, err
	}
	if res == nil || res.Stream == nil {
		return nil, nil, fmt.Errorf("nil stream result")
	}

	// If the LLM client generated a different ID (shouldn't happen if it reuses
	// the one we attached), prefer the returned ID.
	if res.InvocationID != "" {
		prep.InvocationID = res.InvocationID
	}

	return prep, res.Stream, nil
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
