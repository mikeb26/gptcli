/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

type CreateFileTool struct {
	approver am.Approver
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

func commonFileBuildApprovalRequest(t types.Tool, arg any, filenameIn string,
	writeRequired bool) am.ApprovalRequest {

	// Normalize to an absolute, cleaned path so that approval policies
	// are keyed consistently regardless of how the tool was invoked
	// (relative vs absolute paths). This ensures that cached approvals
	// for a given file or directory apply across different invocations.
	filename := filenameIn
	if !filepath.IsAbs(filenameIn) {
		if abs, err := filepath.Abs(filenameIn); err == nil {
			filename = abs
		} else {
			filename = filepath.Clean(filenameIn)
		}
	} else {
		filename = filepath.Clean(filenameIn)
	}
	isDir := false
	fInfo, err := os.Stat(filename)
	if err == nil {
		isDir = fInfo.IsDir()
	}
	var dirname string
	if isDir {
		dirname = filename
	} else {
		dirname = filepath.Dir(filename)
	}

	var dirPolicyID string
	if dirname != string(filepath.Separator) && dirname != "." && dirname != "" {
		dirPolicyID = am.ApprovalPolicyID(am.ApprovalSubsysTools,
			am.ApprovalGroupFileIO, am.ApprovalTargetDir, dirname)
	}

	filePolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupFileIO, am.ApprovalTargetFile, filename)

	promptBuilder := &strings.Builder{}
	promptBuilder.WriteString(fmt.Sprintf("gptcli would like to %v:%v. Allow?",
		t.GetOp(), filename))

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		}}
	if !isDir {
		if !writeRequired {
			choices = append(choices, am.ApprovalChoice{
				Key:      "fr",
				Label:    "Yes, and allow all future reads of this file",
				Scope:    am.ApprovalScopeTarget,
				PolicyID: filePolicyID,
				Actions:  []am.ApprovalAction{am.ApprovalActionRead},
			})
		}

		choices = append(choices, am.ApprovalChoice{
			Key:      "fw",
			Label:    "Yes, and allow all future reads/writes of this file",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: filePolicyID,
			Actions: []am.ApprovalAction{am.ApprovalActionWrite,
				am.ApprovalActionRead},
		})
	}

	if dirPolicyID != "" {
		if !writeRequired {
			choices = append(choices, am.ApprovalChoice{
				Key:      "dr",
				Label:    fmt.Sprintf("Yes, and allow all future reads within %v (recursively)", dirname),
				Scope:    am.ApprovalScopeTarget,
				PolicyID: dirPolicyID,
				Actions:  []am.ApprovalAction{am.ApprovalActionRead},
			})
		}
		choices = append(choices, am.ApprovalChoice{
			Key:      "dw",
			Label:    fmt.Sprintf("Yes, and allow all future reads/writes within %v (recursively)", dirname),
			Scope:    am.ApprovalScopeTarget,
			PolicyID: dirPolicyID,
			Actions:  []am.ApprovalAction{am.ApprovalActionWrite, am.ApprovalActionRead},
		})
	}

	choices = append(choices, am.ApprovalChoice{
		Key:   "n",
		Label: "No",
		Scope: am.ApprovalScopeDeny,
	})

	return am.ApprovalRequest{
		Prompt:          promptBuilder.String(),
		RequiredActions: []am.ApprovalAction{am.ApprovalActionWrite},
		Choices:         choices,
	}
}

// BuildApprovalRequest implements ToolWithCustomApproval for
// CreateFileTool so that write permissions can be cached on a per-file
// or per-directory basis using ApprovalActionWrite.
func (t CreateFileTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*CreateFileReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	return commonFileBuildApprovalRequest(t, arg, req.Filename, true)
}

type AppendFileTool struct {
	approver am.Approver
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// AppendFileTool so that write permissions can be cached similarly to
// CreateFileTool.
func (t AppendFileTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*AppendFileReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	return commonFileBuildApprovalRequest(t, arg, req.Filename, true)
}

type ReadFileTool struct {
	approver am.Approver
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

// BuildApprovalRequest implements ToolWithCustomApproval to provide
// file- and directory-specific approval prompts and options. It supports
// granting approval for a single read, for a specific file, or for all
// reads within a directory tree (recursively).
func (t ReadFileTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*ReadFileReq)
	if !ok || req == nil {
		// Fallback to the default behavior if the argument is not as
		// expected; this should not happen in normal flows.
		return DefaultApprovalRequest(t, arg)
	}

	return commonFileBuildApprovalRequest(t, arg, req.Filename, false)
}

type DeleteFileTool struct {
	approver am.Approver
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// DeleteFileTool so that write permissions can be cached consistently
// with CreateFileTool and AppendFileTool.
func (t DeleteFileTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*DeleteFileReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	return commonFileBuildApprovalRequest(t, arg, req.Filename, true)
}

func NewDeleteFileTool(approver am.Approver) types.LlmTool {
	t := &DeleteFileTool{
		approver: approver,
	}

	return t.Define()
}

func NewReadFileTool(approver am.Approver) types.LlmTool {
	t := &ReadFileTool{
		approver: approver,
	}

	return t.Define()
}

func (t ReadFileTool) Define() types.LlmTool {
	ret, err := utils.InferTool(string(t.GetOp()), "read a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t DeleteFileTool) Define() types.LlmTool {
	ret, err := utils.InferTool(string(t.GetOp()), "delete a file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewAppendFileTool(approver am.Approver) types.LlmTool {
	t := &AppendFileTool{
		approver: approver,
	}

	return t.Define()
}

func (t AppendFileTool) Define() types.LlmTool {
	ret, err := utils.InferTool(string(t.GetOp()), "append to an existing file on the local filesystem",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func NewCreateFileTool(approver am.Approver) types.LlmTool {
	t := &CreateFileTool{
		approver: approver,
	}

	return t.Define()
}

func (t CreateFileTool) Define() types.LlmTool {
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

	err := GetUserApproval(ctx, t.approver, t, req)
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

	err := GetUserApproval(ctx, t.approver, t, req)
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

	err := GetUserApproval(ctx, t.approver, t, req)
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

	err := GetUserApproval(ctx, t.approver, t, req)
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
