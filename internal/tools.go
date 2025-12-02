/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"context"
	"fmt"

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
	// GetUI gets the underlying ui component that the approval ui was built
	// from
	GetUI() types.GptCliUI
}

// StdioApprovalUI is a ToolApprovalUI implementation that uses a bufio.Reader
// for input and an io.Writer for output (typically os.Stdout).
type approvalUI struct {
	ui types.GptCliUI
}

func newApprovalUI(uiIn types.GptCliUI) *approvalUI {
	return &approvalUI{ui: uiIn}
}

func (aui *approvalUI) GetUI() types.GptCliUI {
	return aui.ui
}

func (aui *approvalUI) AskApproval(t types.Tool, arg any) (bool, error) {

	prompt := fmt.Sprintf("gptcli would like to '%v'('%v')\nallow?",
		t.GetOp(), arg)
	trueOpt := types.GptCliUIOption{Key: "y", Label: "y"}
	falseOpt := types.GptCliUIOption{Key: "n", Label: "n"}

	return aui.ui.SelectBool(prompt+" (y/n): ", trueOpt, falseOpt, nil)
}

func defineTools(ctx context.Context, vendor string, ui types.GptCliUI,
	apiKey string, model string, depth int) []types.GptCliTool {

	approvalUI := newApprovalUI(ui)
	tools := []types.GptCliTool{
		NewRunCommandTool(approvalUI),
		NewCreateFileTool(approvalUI),
		NewAppendFileTool(approvalUI),
		NewFilePatchTool(approvalUI),
		NewReadFileTool(approvalUI),
		NewDeleteFileTool(approvalUI),
		NewPwdTool(approvalUI),
		NewChdirTool(approvalUI),
		NewEnvGetTool(approvalUI),
		NewEnvSetTool(approvalUI),
		NewRetrieveUrlTool(approvalUI),
		NewRenderWebTool(approvalUI),
	}
	if depth <= MaxDepth {
		tools = append(tools, NewPromptRunTool(ctx, vendor, approvalUI, apiKey,
			model, depth))
	}

	return tools
}

// getUserApproval is a helper that enforces the RequiresUserApproval contract
// and delegates the actual interaction to the provided ToolApprovalUI.
func getUserApproval(ui ToolApprovalUI, t types.Tool, arg any) error {
	if !t.RequiresUserApproval() {
		return nil
	}

	ok, err := ui.AskApproval(t, arg)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("The user denied approval for us to run %v(%v); you(the AI agent) should provide justification to the gptcli user for why we need to invoke it.",
			t.GetOp(), arg)
	}

	return nil
}
