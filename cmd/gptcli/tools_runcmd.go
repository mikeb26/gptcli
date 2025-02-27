/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type RunCommandTool struct{}

type CmdRunResp struct {
	Err    error  `json:"error"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

func (t RunCommandTool) GetOp() ToolCallOp {
	return RunCommand
}

func (t RunCommandTool) RequiresUserApproval() bool {
	return true
}

func (RunCommandTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"cmd": {
				Type:        jsonschema.String,
				Description: "The command to execute.",
			},
			"cmdArgs": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: "A list of arguments to include when running the command.",
			},
		},
		Required: []string{"cmd"},
	}
	f := openai.FunctionDefinition{
		Name:        string(RunCommand),
		Description: "Run a command on the user's behalf",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (t RunCommandTool) Invoke(args map[string]any) (string, error) {
	cmdStr, ok := args["cmd"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'cmd' arg")
	}
	cmdArgsIf, ok := args["cmdArgs"].([]interface{})
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'cmdArgs' args")
	}
	cmdArgs := make([]string, len(cmdArgsIf))
	for i, v := range cmdArgsIf {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("gptcli: unable to parse '%v' in cmdArgs", v)
		}
		cmdArgs[i] = s
	}

	var resp CmdRunResp

	cmd := exec.Command(cmdStr, cmdArgs...)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin

	var stdoutSb strings.Builder
	var stderrSb strings.Builder

	cmd.Stdout = &stdoutSb
	cmd.Stderr = &stderrSb

	resp.Err = cmd.Run()
	resp.Stderr = stderrSb.String()
	resp.Stdout = stdoutSb.String()

	encodedResp, err := json.Marshal(resp)

	return string(encodedResp), err
}
