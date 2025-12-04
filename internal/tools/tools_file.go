/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"os"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type CreateFileTool struct {
	approvalUI ToolApprovalUI
}

type CreateFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to create or overwrite"`
	Content  string `json:"content" jsonschema:"description=The content of the file"`
}

type CreateFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the create call"`
}

func (g CreateFileTool) GetOp() types.ToolCallOp {
	return types.CreateFile
}

func (t CreateFileTool) RequiresUserApproval() bool {
	return true
}

type AppendFileTool struct {
	approvalUI ToolApprovalUI
}

type AppendFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The name of the file to append"`
	Content  string `json:"content" jsonschema:"description=The content to append"`
}

type AppendFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the append call"`
}

func (t AppendFileTool) GetOp() types.ToolCallOp {
	return types.AppendFile
}
func (t AppendFileTool) RequiresUserApproval() bool {
	return true
}

type ReadFileTool struct {
	approvalUI ToolApprovalUI
}

type ReadFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The file to read"`
}

type ReadFileResp struct {
	Error   string `json:"error" jsonschema:"description=The error status of the read call"`
	Content string `json:"content" jsonschema:"description=The content of the file"`
}

func (t ReadFileTool) GetOp() types.ToolCallOp {
	return types.ReadFile
}

func (t ReadFileTool) RequiresUserApproval() bool {
	return true
}

type DeleteFileTool struct {
	approvalUI ToolApprovalUI
}

type DeleteFileReq struct {
	Filename string `json:"filename" jsonschema:"description=The file to delete"`
}

type DeleteFileResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the delete call"`
}

func (t DeleteFileTool) GetOp() types.ToolCallOp {
	return types.DeleteFile
}

func (t DeleteFileTool) RequiresUserApproval() bool {
	return true
}

func NewDeleteFileTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &DeleteFileTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func NewReadFileTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &ReadFileTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t ReadFileTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "read a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t DeleteFileTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "delete a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewAppendFileTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &AppendFileTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t AppendFileTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "append to an existing file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewCreateFileTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &CreateFileTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t CreateFileTool) Define() types.GptCliTool {
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

	err := GetUserApproval(t.approvalUI, t, req)
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

	err := GetUserApproval(t.approvalUI, t, req)
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

	err := GetUserApproval(t.approvalUI, t, req)
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

func (t DeleteFileTool) Invoke(ctx context.Context,
	req *DeleteFileReq) (*DeleteFileResp, error) {

	ret := &DeleteFileResp{}

	err := GetUserApproval(t.approvalUI, t, req)
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
