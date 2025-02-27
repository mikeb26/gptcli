/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/sashabaranov/go-openai"
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
	PromptRun              = "prompt_run"
)

type Tool interface {
	GetOp() ToolCallOp
	RequiresUserApproval() bool
	Invoke(args map[string]any) (string, error)
	Define() openai.Tool
}

var toolInfo = map[ToolCallOp]Tool{
	RunCommand:  RunCommandTool{},
	CreateFile:  CreateFileTool{},
	AppendFile:  AppendFileTool{},
	ReadFile:    ReadFileTool{},
	DeleteFile:  DeleteFileTool{},
	Pwd:         PwdTool{},
	Chdir:       ChdirTool{},
	RetrieveUrl: RetrieveUrlTool{},
	EnvGet:      EnvGetTool{},
	EnvSet:      EnvSetTool{},
}

var initOnce sync.Once

func defineTools(ctx context.Context, client *openai.Client,
	input *bufio.Reader) []openai.Tool {

	tools := make([]openai.Tool, 0)

	if client == nil {
		return tools
	}

	initOnce.Do(func() {
		toolInfo[PromptRun] = NewPromptRunTool(ctx, client, input)
	})

	for _, tool := range toolInfo {
		tools = append(tools, tool.Define())
	}

	return tools
}

func processToolCall(tc openai.ToolCall,
	input *bufio.Reader) (openai.ChatCompletionMessage, error) {

	var err error
	msg := openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    "",
		Name:       tc.Function.Name,
		ToolCallID: tc.ID,
	}

	toolEntry, ok := toolInfo[ToolCallOp(tc.Function.Name)]
	if !ok {
		err = fmt.Errorf("gptcli: Unrecognized tool '%v' args: '%v'",
			tc.Function.Name, tc.Function.Arguments)
		msg.Content = fmt.Sprintf("%v", err)
		return msg, err
	}

	if toolEntry.RequiresUserApproval() {
		fmt.Printf("gptcli would like to '%v'('%v')\n", tc.Function.Name,
			tc.Function.Arguments)

		fmt.Printf("allow? (Y/N) [N]: ")
		allowTool, err := input.ReadString('\n')
		if err != nil {
			return msg, err
		}

		allowTool = strings.ToUpper(strings.TrimSpace(allowTool))
		if len(allowTool) == 0 {
			allowTool = "N"
		}

		if allowTool != "Y" {
			msg.Content = fmt.Sprintf("the user denied your request to invoke tool:%v args:%v. try explaining why you need the tool and asking the user to try again.",
				tc.Function.Name, tc.Function.Arguments)
			return msg, nil
		}
	}

	var args map[string]any
	err = json.Unmarshal([]byte(tc.Function.Arguments), &args)
	if err != nil {
		return msg, fmt.Errorf("gptcli: failed to parse tool '%v' args '%v': %w",
			tc.Function.Name, tc.Function.Arguments, err)
	}

	msg.Content, err = toolEntry.Invoke(args)
	if err != nil && msg.Content == "" {
		msg.Content = fmt.Sprintf("%v", err)
	} else if err == nil && msg.Content == "" {
		msg.Content = "success"
	}

	return msg, err
}
