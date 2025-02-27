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

type CreateFileTool struct{}

func (t CreateFileTool) GetOp() ToolCallOp {
	return CreateFile
}

func (t CreateFileTool) RequiresUserApproval() bool {
	return true
}

type AppendFileTool struct{}

func (t AppendFileTool) GetOp() ToolCallOp {
	return AppendFile
}
func (t AppendFileTool) RequiresUserApproval() bool {
	return true
}

type ReadFileTool struct{}

func (t ReadFileTool) GetOp() ToolCallOp {
	return ReadFile
}

func (t ReadFileTool) RequiresUserApproval() bool {
	return true
}

type DeleteFileTool struct{}

func (t DeleteFileTool) GetOp() ToolCallOp {
	return DeleteFile
}

func (t DeleteFileTool) RequiresUserApproval() bool {
	return true
}

func (ReadFileTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file_name": {
				Type:        jsonschema.String,
				Description: "The file to read",
			},
		},
		Required: []string{"file_name"},
	}
	f := openai.FunctionDefinition{
		Name:        string(ReadFile),
		Description: "read a file",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (DeleteFileTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file_name": {
				Type:        jsonschema.String,
				Description: "The file to delete",
			},
		},
		Required: []string{"file_name"},
	}
	f := openai.FunctionDefinition{
		Name:        string(DeleteFile),
		Description: "delete a file",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (AppendFileTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file_name": {
				Type:        jsonschema.String,
				Description: "The existing file to append to",
			},
			"content": {
				Type:        jsonschema.String,
				Description: "The content to append to the file",
			},
		},
		Required: []string{"file_name", "content"},
	}
	f := openai.FunctionDefinition{
		Name:        string(AppendFile),
		Description: "append to an existing file",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (CreateFileTool) Define() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"file_name": {
				Type:        jsonschema.String,
				Description: "The new file to create",
			},
			"content": {
				Type:        jsonschema.String,
				Description: "The content to create the new file with",
			},
		},
		Required: []string{"file_name", "content"},
	}
	f := openai.FunctionDefinition{
		Name:        string(CreateFile),
		Description: "create a new file",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func (CreateFileTool) Invoke(args map[string]any) (string, error) {
	fileName, ok := args["file_name"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'file_name' arg")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'content' arg")
	}
	err := os.WriteFile(fileName, []byte(content), 0644)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to write '%v': %w", fileName, err)
	}

	return "", nil
}

func (AppendFileTool) Invoke(args map[string]any) (string, error) {
	fileName, ok := args["file_name"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'file_name' arg")
	}
	content, ok := args["content"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'content' arg")
	}
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to open '%v': %w", fileName, err)
	}
	defer file.Close()
	_, err = file.WriteString(content)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to write '%v': %w", fileName, err)
	}

	return "", nil
}

func (ReadFileTool) Invoke(args map[string]any) (string, error) {
	fileName, ok := args["file_name"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'file_name' arg")
	}
	content, err := os.ReadFile(fileName)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to read '%v': %w", fileName, err)
	}

	return string(content), nil
}

func (DeleteFileTool) Invoke(args map[string]any) (string, error) {
	fileName, ok := args["file_name"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'file_name' arg")
	}
	err := os.Remove(fileName)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to remove '%v': %w", fileName, err)
	}

	return "", nil
}
