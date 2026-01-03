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
	Thread          *Thread
	FullDialogue    []*types.ThreadMessage // full history + user request
	WorkingDialogue []*types.ThreadMessage // possibly summarized + user request
	ReqMsg          *types.ThreadMessage
	InvocationID    string
}

// setCurrentThreadRunning() returns the currently selected thread, if any,
// and sets it to a running sttate
//
// NOTE: Callers that need a stable reference for the lifetime of a request
// should call this once and hold on to the returned pointer; callers should
// not repeatedly consult "current thread" state from the thread group.
func (thrGrp *ThreadGroup) setCurrentThreadRunning() (*Thread, error) {
	thrGrp.mu.RLock()
	defer thrGrp.mu.RUnlock()

	if thrGrp.curThreadNum == 0 || thrGrp.curThreadNum > thrGrp.totThreads {
		return nil, fmt.Errorf("No thread is currently selected.")
	}

	thr := thrGrp.threads[thrGrp.curThreadNum-1]

	thr.mu.Lock()
	defer thr.mu.Unlock()

	if thr.state != ThreadStateIdle {
		return nil, fmt.Errorf("cannot set non-idle thread to running state:%v",
			thr.state)
	}

	thr.state = ThreadStateRunning

	return thr, nil

}

// prepareChatOnceInThread performs all work needed before sending a request to
// the LLM for a specific thread: validating inputs, constructing the user
// message, optionally summarizing prior dialogue, and returning both the full
// and working dialogue slices.
//
// This method intentionally does not consult thrGrp's notion of "current
// thread" so that callers can safely record a thread pointer once and reuse it
// for the lifetime of a run.
func (thrGrp *ThreadGroup) prepareChatOnceInThread(
	ctx context.Context, llmClient types.AIClient, thread *Thread,
	prompt string, summarizePrior bool) (*PreparedChat, error) {

	reqMsg := &types.ThreadMessage{
		Role:    types.LlmRoleUser,
		Content: prompt,
	}

	// Attach a thread-state setter so lower layers (e.g. tool approval prompts)
	// can signal when the active thread is blocked waiting on user interaction
	// without creating an import cycle.
	ctx = WithThread(ctx, thread)

	// Copy the dialogue slice so that preparing a request does not mutate the
	// thread's in-memory dialogue (and is safer under concurrent reads).
	thread.mu.RLock()
	fullDialogue := make([]*types.ThreadMessage, len(thread.persisted.Dialogue))
	copy(fullDialogue, thread.persisted.Dialogue)
	thread.mu.RUnlock()

	fullDialogue = append(fullDialogue, reqMsg)
	workingDialogue := fullDialogue

	var err error
	if summarizePrior && len(fullDialogue) > 2 {
		// Summarize only the prior dialogue (exclude the current user request).
		prior := fullDialogue[:len(fullDialogue)-1]
		summaryDialogue, sumErr := summarizeDialogue(ctx, llmClient, prior)
		if sumErr != nil {
			return nil, sumErr
		}
		workingDialogue = append(summaryDialogue, reqMsg)
	}

	prep := &PreparedChat{
		Ctx:             ctx,
		Thread:          thread,
		FullDialogue:    fullDialogue,
		WorkingDialogue: workingDialogue,
		ReqMsg:          reqMsg,
	}

	return prep, err
}

// FinalizeChatOnce appends the assistant reply to the
// thread's dialogue, updates timestamps, and persists the thread to
// disk.
func (thrGrp *ThreadGroup) finalizeChatOnce(
	prep *PreparedChat, replyMsg *types.ThreadMessage,
) error {
	if prep == nil || prep.Thread == nil {
		return fmt.Errorf("invalid prepared chat: missing thread")
	}

	thread := prep.Thread
	fullDialogue := append(prep.FullDialogue, replyMsg)

	thread.mu.Lock()
	defer thread.mu.Unlock()

	thread.persisted.Dialogue = fullDialogue
	thread.persisted.ModTime = time.Now()
	thread.persisted.AccessTime = time.Now()
	thread.state = ThreadStateIdle

	if err := thread.save(thrGrp.dir); err != nil {
		return err
	}

	return nil
}

// ChatOnceStream prepares the current thread dialogue
// for a new user prompt and returns both the PreparedChat and a
// streaming reader for the assistant reply. Callers are responsible for
// consuming the stream, assembling the final reply message, and then
// invoking FinalizeChatOnce.
func (thrGrp *ThreadGroup) chatOnceStreamInThread(
	ctx context.Context, llmClient types.AIClient, thread *Thread, prompt string,
	summarizePrior bool,
) (*PreparedChat, *schema.StreamReader[*types.ThreadMessage], error) {

	prep, err := thrGrp.prepareChatOnceInThread(ctx, llmClient, thread, prompt,
		summarizePrior)
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
func summarizeDialogue(ctx context.Context, llmClient types.AIClient,
	dialogue []*types.ThreadMessage) ([]*types.ThreadMessage, error) {

	summaryDialogue := []*types.ThreadMessage{
		{Role: types.LlmRoleSystem,
			Content: prompts.SystemMsg},
	}

	msg := &types.ThreadMessage{
		Role:    types.LlmRoleSystem,
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
