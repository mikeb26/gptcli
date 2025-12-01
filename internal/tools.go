/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

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
}

// StdioApprovalUI is a ToolApprovalUI implementation that uses a bufio.Reader
// for input and an io.Writer for output (typically os.Stdout).
type StdioApprovalUI struct {
	mu  sync.Mutex
	in  *bufio.Reader
	out io.Writer
}

func NewStdioApprovalUI(in *bufio.Reader, out io.Writer) *StdioApprovalUI {
	return &StdioApprovalUI{in: in, out: out}
}

func (ui *StdioApprovalUI) AskApproval(t types.Tool, arg any) (bool, error) {
	ui.mu.Lock()
	defer ui.mu.Unlock()

	if _, err := fmt.Fprintf(ui.out, "gptcli would like to '%v'('%v')\n", t.GetOp(), arg); err != nil {
		return false, err
	}
	if _, err := fmt.Fprintf(ui.out, "allow? (Y/N) [N]: "); err != nil {
		return false, err
	}

	line, err := ui.in.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("gptcli was unable to read user input: %w", err)
	}

	s := strings.ToUpper(strings.TrimSpace(line))
	if s == "" {
		s = "N"
	}

	return s[0] == 'Y', nil
}

func defineTools(ctx context.Context, vendor string, approvalUI ToolApprovalUI,
	apiKey string, model string, depth int) []types.GptCliTool {

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
