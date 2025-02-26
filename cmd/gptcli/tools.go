/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
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
)

type ToolInfo struct {
	Op                  ToolCallOp
	RequireUserApproval bool
	ToolFunc            func(map[string]any) (string, error)
}

var toolInfo = map[ToolCallOp]ToolInfo{
	RunCommand: {Op: RunCommand, RequireUserApproval: true,
		ToolFunc: execCmdWithArgs},
	CreateFile: {Op: CreateFile, RequireUserApproval: true,
		ToolFunc: createFile},
	AppendFile: {Op: AppendFile, RequireUserApproval: true,
		ToolFunc: appendFile},
	ReadFile: {Op: ReadFile, RequireUserApproval: true, ToolFunc: readFile},
	DeleteFile: {Op: DeleteFile, RequireUserApproval: true,
		ToolFunc: deleteFile},
	Pwd:   {Op: Pwd, RequireUserApproval: false, ToolFunc: pwd},
	Chdir: {Op: Chdir, RequireUserApproval: true, ToolFunc: chdir},
	RetrieveUrl: {Op: RetrieveUrl, RequireUserApproval: false,
		ToolFunc: readUrl},
	EnvGet: {Op: EnvGet, RequireUserApproval: false, ToolFunc: getenv},
	EnvSet: {Op: EnvSet, RequireUserApproval: true, ToolFunc: setenv},
}

type CmdRunResp struct {
	Err    error  `json:"error"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
}

type RetrieveUrlResponse struct {
	Status        string
	StatusCode    int
	Proto         string
	ProtoMajor    int
	ProtoMinor    int
	Header        http.Header
	Body          string
	ContentLength int64
}

func defineTools() []openai.Tool {
	tools := make([]openai.Tool, 0)

	tools = append(tools, defineRunCmdTool())
	tools = append(tools, defineCreateFileTool())
	tools = append(tools, defineAppendFileTool())
	tools = append(tools, defineReadFileTool())
	tools = append(tools, defineDeleteFileTool())
	tools = append(tools, definePwdTool())
	tools = append(tools, defineChdirTool())
	tools = append(tools, defineRetrieveUrlTool())
	tools = append(tools, defineEnvGetTool())
	tools = append(tools, defineEnvSetTool())

	return tools
}

func defineRunCmdTool() openai.Tool {
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

func defineReadFileTool() openai.Tool {
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

func defineDeleteFileTool() openai.Tool {
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

func defineRetrieveUrlTool() openai.Tool {
	params := jsonschema.Definition{
		Type: jsonschema.Object,
		Properties: map[string]jsonschema.Definition{
			"url": {
				Type:        jsonschema.String,
				Description: "The url to retrieve",
			},
			"headers": {
				Type: jsonschema.Array,
				Items: &jsonschema.Definition{
					Type: jsonschema.String,
				},
				Description: "Optional request headers as an array of strings formatted as 'Key: Value'",
			},
			"method": {
				Type:        jsonschema.String,
				Description: "Optional HTTP request method (e.g., GET, POST, etc.); defaults to GET",
			},
			"body": {
				Type:        jsonschema.String,
				Description: "Optional HTTP request body",
			},
		},
		Required: []string{"url"},
	}
	f := openai.FunctionDefinition{
		Name:        string(RetrieveUrl),
		Description: "retrieve the content of a url",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func definePwdTool() openai.Tool {
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

func defineChdirTool() openai.Tool {
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

func defineEnvGetTool() openai.Tool {
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

func defineEnvSetTool() openai.Tool {
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

func defineAppendFileTool() openai.Tool {
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
		Description: "append to an exiting file",
		Parameters:  params,
	}
	t := openai.Tool{
		Type:     openai.ToolTypeFunction,
		Function: &f,
	}

	return t
}

func defineCreateFileTool() openai.Tool {
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

func (gptCliCtx *GptCliContext) processToolCall(tc openai.ToolCall) (openai.ChatCompletionMessage, error) {

	var err error
	msg := openai.ChatCompletionMessage{
		Role:       openai.ChatMessageRoleTool,
		Content:    "",
		Name:       tc.Function.Name,
		ToolCallID: tc.ID,
	}

	infoEntry, ok := toolInfo[ToolCallOp(tc.Function.Name)]
	if !ok {
		err = fmt.Errorf("gptcli: Unrecognized tool '%v' args: '%v'",
			tc.Function.Name, tc.Function.Arguments)
		msg.Content = fmt.Sprintf("%v", err)
		return msg, err
	}

	if infoEntry.RequireUserApproval {
		fmt.Printf("gptcli would like to '%v'('%v')\n", tc.Function.Name,
			tc.Function.Arguments)

		fmt.Printf("allow? (Y/N) [N]: ")
		allowTool, err := gptCliCtx.input.ReadString('\n')
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

	msg.Content, err = infoEntry.ToolFunc(args)
	if err != nil && msg.Content == "" {
		msg.Content = fmt.Sprintf("%v", err)
	} else if err == nil && msg.Content == "" {
		msg.Content = "success"
	}

	return msg, err
}

func execCmdWithArgs(args map[string]any) (string, error) {
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

func createFile(args map[string]any) (string, error) {
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

func appendFile(args map[string]any) (string, error) {
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

func readFile(args map[string]any) (string, error) {
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

func readUrl(args map[string]any) (string, error) {
	url, ok := args["url"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'url' arg")
	}

	requestMethod := "GET"
	m, ok := args["method"].(string)
	if ok && m != "" {
		requestMethod = strings.ToUpper(m)
	}

	// Check for an optional body parameter
	var bodyReader io.Reader
	bodyVal, ok := args["body"].(string)
	if ok && bodyVal != "" {
		bodyReader = strings.NewReader(bodyVal)
	}

	httpClient := &http.Client{
		Timeout: time.Second * 30,
	}

	var req *http.Request
	var resp *http.Response
	var err error

	req, err = http.NewRequest(requestMethod, url, bodyReader)
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to create request for '%v' with method '%v': %w", url, requestMethod, err)
	}

	headersArg, ok := args["headers"]
	if ok {
		headersArray, ok := headersArg.([]interface{})
		if !ok {
			return "", fmt.Errorf("gptcli: 'headers' should be an array of strings")
		}
		for _, headerVal := range headersArray {
			headerStr, ok := headerVal.(string)
			if !ok {
				return "", fmt.Errorf("gptcli: each header should be a string")
			}
			parts := strings.SplitN(headerStr, ":", 2)
			if len(parts) != 2 {
				return "", fmt.Errorf("gptcli: invalid header format '%v', expected 'Key: Value'", headerStr)
			}
			headerKey := strings.TrimSpace(parts[0])
			headerValue := strings.TrimSpace(parts[1])
			req.Header.Add(headerKey, headerValue)
		}
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%v: failed to fetch '%v': %w", RetrieveUrl, url, err)
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("%v: failed to read '%v': %w", RetrieveUrl, url, err)
	}

	ret := RetrieveUrlResponse{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        resp.Header,
		Body:          string(content),
		ContentLength: resp.ContentLength,
	}

	encodedRet, err := json.Marshal(ret)
	return string(encodedRet), err
}

func deleteFile(args map[string]any) (string, error) {
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

func pwd(args map[string]any) (string, error) {
	curDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("gptcli: failed to get working directory: %w", err)
	}

	return curDir, nil
}

func chdir(args map[string]any) (string, error) {
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

func getenv(args map[string]any) (string, error) {
	envvar, ok := args["envvar"].(string)
	if !ok {
		return "", fmt.Errorf("gptcli: missing 'envvar' arg")
	}
	ret := os.Getenv(envvar)
	return ret, nil
}

func setenv(args map[string]any) (string, error) {
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
