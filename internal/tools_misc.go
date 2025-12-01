/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package internal

import (
	"context"
	"os"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/types"
)

type PwdTool struct {
	approvalUI ToolApprovalUI
}

type PwdReq struct {
}

type PwdResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the pwd call"`
	Pwd   string `json:"error" jsonschema:"description=The present working directory"`
}

type ChdirTool struct {
	approvalUI ToolApprovalUI
}

type ChdirReq struct {
	Newdir string `json:"error" jsonschema:"description=The new directory to change into"`
}

type ChdirResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the chdir call"`
}

type EnvGetTool struct {
	approvalUI ToolApprovalUI
}

type EnvGetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to get"`
}

type EnvGetResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the envget call"`
	Value string `json:"error" jsonschema:"description=The current value of the request environment variable"`
}

type EnvSetTool struct {
	approvalUI ToolApprovalUI
}

type EnvSetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to set"`
	Value  string `json:"error" jsonschema:"description=The new value to set"`
}

type EnvSetResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the envset call"`
}

func (t PwdTool) GetOp() types.ToolCallOp {
	return types.Pwd
}

func (t PwdTool) RequiresUserApproval() bool {
	return false
}
func NewPwdTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &PwdTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t PwdTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "print the current working directory",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t PwdTool) Invoke(ctx context.Context, _ *PwdReq) (*PwdResp, error) {
	ret := &PwdResp{}

	err := getUserApproval(t.approvalUI, t, "")
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	curDir, err := os.Getwd()
	if err != nil {
		ret.Error = err.Error()
	} else {
		ret.Pwd = curDir
	}

	return ret, nil
}

func (t ChdirTool) GetOp() types.ToolCallOp {
	return types.Chdir
}

func (t ChdirTool) RequiresUserApproval() bool {
	return true
}
func NewChdirTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &ChdirTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t ChdirTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "change the current working directory",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t ChdirTool) Invoke(ctx context.Context,
	req *ChdirReq) (*ChdirResp, error) {

	ret := &ChdirResp{}

	err := getUserApproval(t.approvalUI, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	err = os.Chdir(req.Newdir)
	if err != nil {
		ret.Error = err.Error()
	}

	return ret, nil
}

func (t EnvGetTool) GetOp() types.ToolCallOp {
	return types.EnvGet
}

func (t EnvGetTool) RequiresUserApproval() bool {
	return true
}
func NewEnvGetTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &EnvGetTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t EnvGetTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "get an environment variable",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t EnvGetTool) Invoke(ctx context.Context,
	req *EnvGetReq) (*EnvGetResp, error) {

	ret := &EnvGetResp{}

	err := getUserApproval(t.approvalUI, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	ret.Value = os.Getenv(req.Envvar)

	return ret, nil
}

func (t EnvSetTool) GetOp() types.ToolCallOp {
	return types.EnvSet
}

func (t EnvSetTool) RequiresUserApproval() bool {
	return true
}
func NewEnvSetTool(approvalUI ToolApprovalUI) types.GptCliTool {
	t := &EnvSetTool{
		approvalUI: approvalUI,
	}

	return t.Define()
}

func (t EnvSetTool) Define() types.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "set an environment variable",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t EnvSetTool) Invoke(ctx context.Context,
	req *EnvSetReq) (*EnvSetResp, error) {

	ret := &EnvSetResp{}

	err := getUserApproval(t.approvalUI, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	err = os.Setenv(req.Envvar, req.Value)
	if err != nil {
		ret.Error = err.Error()
	}

	return ret, nil
}
