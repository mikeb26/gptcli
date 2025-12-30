/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"fmt"

	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

// GetUserApproval is a helper that enforces the RequiresUserApproval contract
// and delegates the actual interaction to the provided ToolApprovalUI.
func GetUserApproval(ctx context.Context, approver am.Approver,
	t types.Tool, arg any) error {

	if !t.RequiresUserApproval() {
		return nil
	}

	var req am.ApprovalRequest
	if ca, ok := t.(ToolWithCustomApproval); ok {
		req = ca.BuildApprovalRequest(arg)
	} else {
		req = DefaultApprovalRequest(t, arg)
	}
	dec, err := approver.AskApproval(ctx, req)
	if err != nil {
		return err
	}
	if !dec.Allowed {
		return fmt.Errorf("The user denied approval for us to run %v(%v); you(the AI agent) should provide justification to the gptcli user for why we need to invoke it.",
			t.GetOp(), arg)
	}

	return nil
}

// ToolWithCustomApproval can be implemented by tools that want to
// customize their approval prompt and options.
type ToolWithCustomApproval interface {
	BuildApprovalRequest(arg any) am.ApprovalRequest
}

// DefaultApprovalRequest builds the legacy yes/no style approval
// request used by tools that do not customize their approvals.
func DefaultApprovalRequest(t types.Tool, arg any) am.ApprovalRequest {
	prompt := fmt.Sprintf("gptcli would like to '%v'('%v')\nallow?", t.GetOp(), arg)
	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "yes",
			Scope: am.ApprovalScopeOnce,
		},
		{
			Key:   "n",
			Label: "no",
			Scope: am.ApprovalScopeDeny,
		},
	}

	return am.ApprovalRequest{
		Prompt:  prompt,
		Choices: choices,
	}
}
