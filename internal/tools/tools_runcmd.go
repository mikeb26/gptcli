/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/am"
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// RunCommandTool to enable richer, cached approvals for OS-level
// command execution. Approvals can be granted for a single
// invocation, for all invocations of a specific command name, or for
// a specific command+argument combination (hashed for brevity).
func (t RunCommandTool) BuildApprovalRequest(arg any) ToolApprovalRequest {
	req, ok := arg.(*CmdRunReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	// Construct stable policy identifiers for the command and the full
	// invocation (command + arguments). The invocation ID uses a hash
	// of the argument vector to keep policy keys manageable while still
	// being specific.
	cmdPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupCommand, am.ApprovalTargetCommand, req.Cmd)

	invocationKey := buildCommandInvocationKey(req.Cmd, req.CmdArgs)
	invocationPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupCommand, am.ApprovalTargetCommandInvocation,
		invocationKey)

	prefixKey := buildCommandInvocationPrefixKey(req.Cmd, req.CmdArgs)
	prefixPolicyID := ""
	if prefixKey != "" {
		prefixPolicyID = am.ApprovalPolicyID(am.ApprovalSubsysTools,
			am.ApprovalGroupCommand, am.ApprovalTargetCommandInvocationPrefix,
			prefixKey)
	}

	prompt := fmt.Sprintf("gptcli would like to run OS command: %q with args %q. Allow?",
		req.Cmd, strings.Join(req.CmdArgs, " "))

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
		{
			Key:      "ci",
			Label:    "Yes, and allow this exact command invocation in the future",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: invocationPolicyID,
			Actions:  []am.ApprovalAction{am.ApprovalActionExecute},
		},
		{
			Key:      "cc",
			Label:    "Yes, and allow any arguments for this command in the future",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: cmdPolicyID,
			Actions:  []am.ApprovalAction{am.ApprovalActionExecute},
		},
		{
			Key:   "n",
			Label: "No",
			Scope: am.ApprovalScopeDeny,
		},
	}

	if prefixPolicyID != "" {
		// Insert the "similar" option after the exact-invocation option.
		// (before the broader "any args for this command" option)
		choices = append(choices[:2], append([]am.ApprovalChoice{
			{
				Key:      "cs",
				Label:    "Yes, and allow similar command invocations in the future",
				Scope:    am.ApprovalScopeTarget,
				PolicyID: prefixPolicyID,
				Actions:  []am.ApprovalAction{am.ApprovalActionExecute},
			},
		}, choices[2:]...)...)
	}

	return ToolApprovalRequest{
		Tool:            t,
		Arg:             arg,
		Prompt:          prompt,
		RequiredActions: []am.ApprovalAction{am.ApprovalActionExecute},
		Choices:         choices,
	}
}

// buildCommandInvocationPrefixKey creates a stable key for a command
// invocation "prefix" where all arguments except the last must match.
//
// Example:
//   cmd="go", args=["test","somepkg"] => key = "go:test"
//
// This returns an empty string when there is no meaningful prefix
// (e.g. zero args).
func buildCommandInvocationPrefixKey(cmd string, args []string) string {
	if len(args) < 2 {
		return ""
	}
	// all but last
	prefixArgs := args[:len(args)-1]
	return cmd + ":" + strings.Join(prefixArgs, "\x00")
}

// buildCommandInvocationKey creates a concise but stable key for a
// command invocation by hashing its argument vector. This allows us to
// persist approvals for a specific command+args pair without storing
// arbitrarily long or sensitive arguments verbatim in the policy ID.
func buildCommandInvocationKey(cmd string, args []string) string {
	joined := strings.Join(args, "\x00")
	h := sha256.Sum256([]byte(joined))
	return fmt.Sprintf("%s:%s", cmd, hex.EncodeToString(h[:8]))
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

	err := GetUserApproval(t.approvalUI, t, req)
	if err != nil {
		resp.Error = err.Error()
		return resp, nil
	}

	cmd := exec.CommandContext(ctx, req.Cmd, req.CmdArgs...)
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
