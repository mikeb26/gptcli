/* Copyright © 2023-2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package ui

import (
	"context"
	"fmt"

	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

type UIApprover struct {
	ui types.GptCliUI
}

func NewUIApprover(
	uiIn types.GptCliUI,
) *UIApprover {

	return &UIApprover{
		ui: uiIn,
	}
}

func (ta *UIApprover) AskApproval(ctx context.Context,
	req am.ApprovalRequest) (am.ApprovalDecision, error) {

	// If the caller attached a thread-state setter to the context, mark the
	// thread blocked while we prompt for user input.
	ta.setThreadBlocked(ctx)
	defer ta.setThreadRunning(ctx)

	choices := make([]types.GptCliUIOption, len(req.Choices))
	for i, ch := range req.Choices {
		choices[i] = types.GptCliUIOption{Key: ch.Key, Label: ch.Label}
	}

	sel, err := ta.ui.SelectOption(req.Prompt+" ", choices)
	if err != nil {
		return am.ApprovalDecision{}, err
	}

	// Find the matching choice
	var chosen am.ApprovalChoice
	found := false
	for _, ch := range req.Choices {
		if ch.Key == sel.Key {
			chosen = ch
			found = true
			break
		}
	}
	if !found {
		return am.ApprovalDecision{}, fmt.Errorf("invalid selection: %v", sel.Key)
	}

	allowed := chosen.Scope != am.ApprovalScopeDeny

	return am.ApprovalDecision{
		Allowed: allowed,
		Choice:  chosen,
	}, nil
}

func (ta *UIApprover) setThreadBlocked(ctx context.Context) {
	setter, ok := types.GetThreadStateSetter(ctx)
	if !ok || setter == nil {
		return
	}
	setter.SetThreadStateBlocked()
}

func (ta *UIApprover) setThreadRunning(ctx context.Context) {
	setter, ok := types.GetThreadStateSetter(ctx)
	if !ok || setter == nil {
		return
	}
	setter.SetThreadStateRunning()
}
