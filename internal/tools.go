/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/mikeb26/gptcli/internal/types"
)

func defineTools(ctx context.Context, vendor string, input *bufio.Reader,
	apiKey string, model string, depth int) []types.GptCliTool {

	tools := []types.GptCliTool{
		NewRunCommandTool(input),
		NewCreateFileTool(input),
		NewAppendFileTool(input),
		NewFilePatchTool(input),
		NewReadFileTool(input),
		NewDeleteFileTool(input),
		NewPwdTool(input),
		NewChdirTool(input),
		NewEnvGetTool(input),
		NewEnvSetTool(input),
		NewRetrieveUrlTool(input),
		NewRenderWebTool(input),
	}
	if depth <= MaxDepth {
		tools = append(tools, NewPromptRunTool(ctx, vendor, input, apiKey,
			model, depth))
	}

	return tools
}

var userApprovalMu sync.Mutex

func getUserApproval(input *bufio.Reader, t types.Tool, arg any) error {
	if !t.RequiresUserApproval() {
		return nil
	}

	userApprovalMu.Lock()
	defer userApprovalMu.Unlock()

	fmt.Printf("gptcli would like to '%v'('%v')\n", t.GetOp(), arg)
	fmt.Printf("allow? (Y/N) [N]: ")
	allowTool, err := input.ReadString('\n')
	if err != nil {
		return fmt.Errorf("gptcli was unable to read user input: %w", err)
	}

	allowTool = strings.ToUpper(strings.TrimSpace(allowTool))
	if len(allowTool) == 0 {
		allowTool = "N"
	}

	if allowTool[0] != 'Y' {
		return fmt.Errorf("The user denied approval for us to run %v(%v); you(the AI agent) should provide justification to the gptcli user for why we need to invoke it.",
			t.GetOp(), arg)
	}

	return nil
}
