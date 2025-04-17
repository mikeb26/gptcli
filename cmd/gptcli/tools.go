/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	"fmt"
	"strings"

	"github.com/mikeb26/gptcli/internal"
)

type ToolCallOp string

const (
	RunCommand  ToolCallOp = "cmd_run"
	CreateFile             = "file_create"
	AppendFile             = "file_append"
	ReadFile               = "file_read"
	DeleteFile             = "file_delete"
	Pwd                    = "dir_pwd"
	Chdir                  = "dir_chdir"
	EnvGet                 = "env_get"
	EnvSet                 = "env_set"
	RetrieveUrl            = "url_retrieve"
	RenderUrl              = "url_render"
	PromptRun              = "prompt_run"
)

type Tool interface {
	GetOp() ToolCallOp
	RequiresUserApproval() bool
	Define() internal.GptCliTool
}

func defineTools(ctx context.Context, vendor string, input *bufio.Reader,
	apiKey string, model string, depth int) []internal.GptCliTool {

	tools := []internal.GptCliTool{
		NewRunCommandTool(input),
		NewCreateFileTool(input),
		NewAppendFileTool(input),
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
		tools = append(tools, NewPromptRunTool(ctx, vendor, input, apiKey, model,
			depth))
	}

	return tools
}

func getUserApproval(input *bufio.Reader, t Tool, arg any) error {
	if !t.RequiresUserApproval() {
		return nil
	}

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
