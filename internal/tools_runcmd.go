/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type RunCommandTool struct {
	input *bufio.Reader
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

func NewRunCommandTool(inputIn *bufio.Reader) types.GptCliTool {
	t := &RunCommandTool{
		input: inputIn,
	}

	return t.Define()
}

func (t RunCommandTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "Run a command on the user's behalf",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t RunCommandTool) Invoke(ctx context.Context,
	req *CmdRunReq) (*CmdRunResp, error) {

	resp := &CmdRunResp{}

	err := getUserApproval(t.input, t, req)
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
