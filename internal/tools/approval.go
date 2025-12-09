/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"fmt"

	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

// ToolApprovalUI abstracts how the user is asked to approve a tool invocation.
//
// Implementations can use stdio, ncurses, GUI, etc. to collect the
// decision; tools and getUserApproval remain UI-agnostic.
type ToolApprovalUI interface {
	// AskApproval should return true if the user approves running this tool
	// with the given argument, false otherwise.
	AskApproval(t types.Tool, arg any) (bool, error)

	// AskApprovalEx is an extended approval call which operates on a richer
	// request/decision model that can support multiple options and policy
	// scopes.
	AskApprovalEx(req ToolApprovalRequest) (ToolApprovalDecision, error)
	// GetUI gets the underlying ui component that the approval ui was built
	// from
	GetUI() types.GptCliUI
}

// StdioApprovalUI is a ToolApprovalUI implementation that uses a bufio.Reader
// for input and an io.Writer for output (typically os.Stdout).
type approvalUI struct {
	ui     types.GptCliUI
	policy am.ApprovalPolicyStore
}

func NewApprovalUI(uiIn types.GptCliUI, store am.ApprovalPolicyStore) *approvalUI {
	return &approvalUI{ui: uiIn, policy: store}
}

func (aui *approvalUI) GetUI() types.GptCliUI {
	return aui.ui
}

func (aui *approvalUI) AskApproval(t types.Tool, arg any) (bool, error) {
	req := DefaultApprovalRequest(t, arg)
	dec, err := aui.AskApprovalEx(req)
	if err != nil {
		return false, err
	}
	return dec.Allowed, nil
}

// AskApprovalEx implements the extended approval interaction using the
// underlying UI's SelectOption method.
func (aui *approvalUI) AskApprovalEx(req ToolApprovalRequest) (ToolApprovalDecision, error) {
	// If we have a policy store and this request declares required
	// actions, try to short-circuit based on cached policy. We only
	// consider choices that have a PolicyID (i.e. are eligible for
	// persistence) and whose cached action set is a superset of the
	// required actions.
	if aui.policy != nil && len(req.RequiredActions) > 0 {
		for _, ch := range req.Choices {
			if ch.PolicyID == "" {
				continue
			}
			if actions, found := aui.policy.Check(ch.PolicyID); found && hasAllApprovalActions(actions, req.RequiredActions) {
				return ToolApprovalDecision{
					Allowed: true,
					Choice:  ch,
				}, nil
			}
		}
	}

	if len(req.Choices) == 0 {
		return ToolApprovalDecision{}, fmt.Errorf("no approval choices provided")
	}

	choices := make([]types.GptCliUIOption, len(req.Choices))
	for i, ch := range req.Choices {
		choices[i] = types.GptCliUIOption{Key: ch.Key, Label: ch.Label}
	}

	sel, err := aui.ui.SelectOption(req.Prompt+" ", choices)
	if err != nil {
		return ToolApprovalDecision{}, err
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
		return ToolApprovalDecision{}, fmt.Errorf("invalid selection: %v", sel.Key)
	}

	allowed := chosen.Scope != am.ApprovalScopeDeny

	// Persist policy when appropriate. We currently only persist allow
	// decisions that specify at least one action. Denials are not
	// persisted, so the user will be prompted again on subsequent
	// invocations.
	if aui.policy != nil && chosen.PolicyID != "" && chosen.Scope == am.ApprovalScopeTarget && allowed && len(chosen.Actions) > 0 {
		aui.policy.Save(chosen.PolicyID, chosen.Actions)
	}

	return ToolApprovalDecision{
		Allowed: allowed,
		Choice:  chosen,
	}, nil
}

// GetUserApproval is a helper that enforces the RequiresUserApproval contract
// and delegates the actual interaction to the provided ToolApprovalUI.
func GetUserApproval(ui ToolApprovalUI, t types.Tool, arg any) error {
	if !t.RequiresUserApproval() {
		return nil
	}

	var req ToolApprovalRequest
	if ca, ok := t.(ToolWithCustomApproval); ok {
		req = ca.BuildApprovalRequest(arg)
	} else {
		req = DefaultApprovalRequest(t, arg)
	}
	dec, err := ui.AskApprovalEx(req)
	if err != nil {
		return err
	}
	if !dec.Allowed {
		return fmt.Errorf("The user denied approval for us to run %v(%v); you(the AI agent) should provide justification to the gptcli user for why we need to invoke it.",
			t.GetOp(), arg)
	}

	return nil
}

// ToolApprovalRequest describes an approval interaction for a tool.
type ToolApprovalRequest struct {
	Tool    types.Tool
	Arg     any
	Prompt  string
	// RequiredActions is the set of actions that must be permitted by a
	// cached policy (if any) in order for this request to be
	// auto-approved without prompting the user.
	RequiredActions []am.ApprovalAction
	Choices         []am.ApprovalChoice
}

// ToolApprovalDecision captures the user's decision from an approval
// interaction.
type ToolApprovalDecision struct {
	Allowed bool
	Choice  am.ApprovalChoice
}

// ToolWithCustomApproval can be implemented by tools that want to
// customize their approval prompt and options.
type ToolWithCustomApproval interface {
	BuildApprovalRequest(arg any) ToolApprovalRequest
}

// DefaultApprovalRequest builds the legacy yes/no style approval
// request used by tools that do not customize their approvals.
func DefaultApprovalRequest(t types.Tool, arg any) ToolApprovalRequest {
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

	return ToolApprovalRequest{
		Tool:    t,
		Arg:     arg,
		Prompt:  prompt,
		Choices: choices,
	}
}

// hasAllApprovalActions reports whether "have" contains all actions in
// "need". Both slices are treated as sets; order and duplicates are
// ignored.
func hasAllApprovalActions(have, need []am.ApprovalAction) bool {
	if len(need) == 0 {
		return true
	}

	set := make(map[am.ApprovalAction]struct{}, len(have))
	for _, a := range have {
		set[a] = struct{}{}
	}
	for _, a := range need {
		if _, ok := set[a]; !ok {
			return false
		}
	}
	return true
}
