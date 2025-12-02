/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type RunCommandTool struct {
	approvalUI ToolApprovalUI
}

type CmdRunReq struct {
	Cmd     string   `json:"cmd" jsonschema:"description=The command to execute"`
	CmdArgs []string `json:"cmdargs" jsonschema:"description=A list of arguments to include when executing the command."`
}

type CmdRunResp struct {
	Error  string `json:"error" jsonschema:"description=The error status of the command"`
	Stdout string `json:"stdout" jsonschema:"description=The standard output emitted by the command"`
	Stderr string `json:"stderr" jsonschema:"description=The standard error emitted by the command"`
}

func (t RunCommandTool) GetOp() types.ToolCallOp {
	return types.RunCommand
}

func (t RunCommandTool) RequiresUserApproval() bool {
	return true
}
func NewRunCommandTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &RunCommandTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t RunCommandTool) Define() types.GptCliTool {
	const cmdRunDesc = "Execute a single OS-level program directly (no shell by default). Do NOT call shell interpreters such as bash, sh, or zsh, and do NOT use `-lc`, unless the user has explicitly requested shell features (pipes, redirects, &&, ||, etc.)."

	ret, err := utils.InferTool(string(t.GetOp()), cmdRunDesc, t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t RunCommandTool) Invoke(ctx context.Context,
	req *CmdRunReq) (*CmdRunResp, error) {

	resp := &CmdRunResp{}

	err := getUserApproval(t.approvalUI, t, req)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	cmd := exec.Command(req.Cmd, req.CmdArgs...)
	cmd.Env = os.Environ()
	// @todo should this be t.input instead of os.Stdin?
	cmd.Stdin = os.Stdin
	var stdoutSb strings.Builder
	var stderrSb strings.Builder
	cmd.Stdout = &stdoutSb
	cmd.Stderr = &stderrSb

	err = cmd.Run()
	if err != nil {
		resp.Error = err.Error()
	}
	resp.Stderr = stderrSb.String()
	resp.Stdout = stdoutSb.String()

	return resp, nil
}
