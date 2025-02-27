/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	_ "embed"
	"fmt"
	"os"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

type PwdTool struct{}
type ChdirTool struct{}
type EnvGetTool struct{}
type EnvSetTool struct{}

func (t PwdTool) GetOp() ToolCallOp {
	return Pwd
}

func (t PwdTool) RequiresUserApproval() bool {
	return false
}

func (PwdTool) Define() openai.Tool {
	f := openai.FunctionDefinition{
		Name:        string(Pwd),
		Description: "print the current working directory",
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (PwdTool) Invoke(args map[string]any) (string, error) {
	curDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to get working directory: %w", err)
	}

	return curDir, nil
}

func (t ChdirTool) GetOp() ToolCallOp {
	return Chdir
}

func (t ChdirTool) RequiresUserApproval() bool {
	return true
}

func (ChdirTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"newdir": {
				Type:        jsonschema.String,
				Description: "The new directory to change into",
			},
		},
		Required: []string{"newdir"},
	}
	f := openai.FunctionDefinition{
		Name:        string(Chdir),
		Description: "change the current working directory",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (t ChdirTool) Invoke(args map[string]any) (string, error) {
	newdir, ok := args["newdir"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'newdir' arg")
	}
	err := os.Chdir(newdir)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to change working directory: %w", err)
	}

	return "", nil
}

func (t EnvGetTool) GetOp() ToolCallOp {
	return EnvGet
}

func (t EnvGetTool) RequiresUserApproval() bool {
	return false
}

func (EnvGetTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"envvar": {
				Type:        jsonschema.String,
				Description: "The environment variable to get",
			},
		},
		Required: []string{"envvar"},
	}
	f := openai.FunctionDefinition{
		Name:        string(EnvGet),
		Description: "get an environment variable",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (EnvGetTool) Invoke(args map[string]any) (string, error) {
	envvar, ok := args["envvar"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'envvar' arg")
	}
	ret := os.Getenv(envvar)
	return ret, nil
}

func (t EnvSetTool) GetOp() ToolCallOp {
	return EnvSet
}

func (t EnvSetTool) RequiresUserApproval() bool {
	return true
}

func (EnvSetTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"envvar": {
				Type:        jsonschema.String,
				Description: "The environment variable to set",
			},
			"value": {
				Type:        jsonschema.String,
				Description: "The value to set",
			},
		},
		Required: []string{"envvar", "value"},
	}
	f := openai.FunctionDefinition{
		Name:        string(EnvSet),
		Description: "set an environment variable",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (EnvSetTool) Invoke(args map[string]any) (string, error) {
	envvar, ok := args["envvar"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'envvar' arg")
	}
	value, ok := args["value"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'value' arg")
	}
	return "", os.Setenv(envvar, value)
}
