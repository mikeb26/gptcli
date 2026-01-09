/* Copyright © 2023-2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package threads

import (
	"context"
	"fmt"
	"time"

	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/llmclient"
	"github.com/mikeb26/gptcli/internal/prompts"
	"github.com/mikeb26/gptcli/internal/types"
)

// setCurrentThreadRunning returns the currently selected thread, if any, and
// transitions it to ThreadStateRunning.
//
// NOTE: Callers that need a stable reference for the lifetime of a request
// should call this once and hold on to the returned pointer; callers should
// not repeatedly consult "current thread" state from the thread group.
func (thrGrp *ThreadGroup) setCurrentThreadRunning(ctx context.Context,
	ictx types.InternalContext) (*thread, error) {
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

	// Create the async approver and LLM client per-thread (and only once per
	// thread).
	if thr.asyncApprover == nil {
		thr.asyncApprover = NewAsyncApprover(ictx.LlmBaseApprover)
	}
	if thr.llmClient == nil {
		approver := am.NewPolicyStoreApprover(thr.asyncApprover,
			ictx.LlmPolicyStore)
		thr.llmClient = llmclient.NewEINOClient(ctx, ictx, approver, 0)
	}

	return thr, nil
}

func finalizeChatOnce(thrGrp *ThreadGroup, thread *thread,
	fullDialogue []*types.ThreadMessage) error {
	if thrGrp == nil || thread == nil {
		return fmt.Errorf("invalid finalize: missing thread group or thread")
	}

	thread.mu.Lock()
	defer thread.mu.Unlock()

	thread.persisted.Dialogue = fullDialogue
	thread.persisted.ModTime = time.Now()
	thread.state = ThreadStateIdle
	thread.runState = nil

	if err := thread.save(thrGrp.dir); err != nil {
		return err
	}

	return nil
}

// summarizeDialogue summarizes the entire chat history in order to reduce LLM
// token costs and refocus the context window.
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
