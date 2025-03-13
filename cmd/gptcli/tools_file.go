/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package main

import (
	"bufio"
	"context"
	"os"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal"
)

type CreateFileTool struct {
	input *bufio.Reader
}

type CreateFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to create or overwrite"`
	Content  string `json:"content" jsonschema:"description=The content of the file"`
}

type CreateFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the create call"`
}

func (g CreateFileTool) GetOp() ToolCallOp {
	return CreateFile
}

func (t CreateFileTool) RequiresUserApproval() bool {
	return true
}

type AppendFileTool struct {
	input *bufio.Reader
}

type AppendFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to append"`
	Content  string `json:"content" jsonschema:"description=The content to append"`
}

type AppendFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the append call"`
}

func (t AppendFileTool) GetOp() ToolCallOp {
	return AppendFile
}
func (t AppendFileTool) RequiresUserApproval() bool {
	return true
}

type ReadFileTool struct {
	input *bufio.Reader
}

type ReadFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The file to read"`
}

type ReadFileResp struct {
	Error   string `json:"error" jsonschema:"description=The error status of the read call"`
	Content string `json:"content" jsonschema:"description=The content of the file"`
}

func (t ReadFileTool) GetOp() ToolCallOp {
	return ReadFile
}

func (t ReadFileTool) RequiresUserApproval() bool {
	return true
}

type DeleteFileTool struct {
	input *bufio.Reader
}

type DeleteFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The file to delete"`
}

type DeleteFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the delete call"`
}

func (t DeleteFileTool) GetOp() ToolCallOp {
	return DeleteFile
}

func (t DeleteFileTool) RequiresUserApproval() bool {
	return true
}

func NewReadFileTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &ReadFileTool{
		input: inputIn,
	}

	return t.Define()
}

func (t ReadFileTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "read a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t DeleteFileTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "delete a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewAppendFileTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &AppendFileTool{
		input: inputIn,
	}

	return t.Define()
}

func (t AppendFileTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "append to an existing file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewCreateFileTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &CreateFileTool{
		input: inputIn,
	}

	return t.Define()
}

func (t CreateFileTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "create or overwrite a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t CreateFileTool) Invoke(ctx context.Context,
	req *CreateFileReq) (*CreateFileResp, error) {

	ret := &CreateFileResp{}

	err := getUserApproval(t.input, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	err = os.WriteFile(req.Filename, []byte(req.Content), 0644)
	if err != nil {
		ret.Error = err.Error()
	}

	return ret, nil
}

func (t AppendFileTool) Invoke(ctx context.Context,
	req *AppendFileReq) (*AppendFileResp, error) {

	ret := &AppendFileResp{}

	err := getUserApproval(t.input, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	file, err := os.OpenFile(req.Filename, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}
	defer file.Close()
	_, err = file.WriteString(req.Content)
	if err != nil {
		ret.Error = err.Error()
	}

	return ret, nil
}

func (t ReadFileTool) Invoke(ctx context.Context,
	req *ReadFileReq) (*ReadFileResp, error) {

	ret := &ReadFileResp{}

	err := getUserApproval(t.input, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	content, err := os.ReadFile(req.Filename)
	if err == nil {
		ret.Content = string(content)
	} else {
		ret.Error = err.Error()
	}

	return ret, nil
}

func NewDeleteFileTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &DeleteFileTool{
		input: inputIn,
	}

	return t.Define()
}

func (t DeleteFileTool) Invoke(ctx context.Context,
	req *DeleteFileReq) (*DeleteFileResp, error) {

	ret := &DeleteFileResp{}

	err := getUserApproval(t.input, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	err = os.Remove(req.Filename)
	if err != nil {
		ret.Error = err.Error()
	}

	return ret, nil
}
