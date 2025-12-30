/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package tools

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/components/tool/utils"
	"github.com/mikeb26/gptcli/internal/am"
	"github.com/mikeb26/gptcli/internal/types"
)

type PwdTool struct {
	approver am.Approver
}

type PwdReq struct {
}

type PwdResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the pwd call"`
	Pwd   string `json:"pwd" jsonschema:"description=The present working directory"`
}

type ChdirTool struct {
	approver am.Approver
}

type ChdirReq struct {
	Newdir string `json:"newdir" jsonschema:"description=The new directory to change into"`
}

type ChdirResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the chdir call"`
}

type EnvGetTool struct {
	approver am.Approver
}

type EnvGetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to get"`
}

type EnvGetResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the envget call"`
	Value string `json:"value" jsonschema:"description=The current value of the request environment variable"`
}

type EnvSetTool struct {
	approver am.Approver
}

type EnvSetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to set"`
	Value  string `json:"value" jsonschema:"description=The new value to set"`
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
func NewPwdTool(approver am.Approver) types.GptCliTool {
	t := &PwdTool{
		approver: approver,
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// ChdirTool so that permissions to change the working directory can be
// cached on a per-directory-tree basis. Approvals can be granted for a
// single chdir, or for a specific target directory and all of its
// subdirectories.
func (t ChdirTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*ChdirReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	// Normalize target directory to an absolute path for consistent
	// policy keys.
	newDir := req.Newdir
	if newDir == "" {
		return DefaultApprovalRequest(t, arg)
	}

	return commonFileBuildApprovalRequest(t, arg, newDir, false)
}
func NewChdirTool(approver am.Approver) types.GptCliTool {
	t := &ChdirTool{
		approver: approver,
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

	err := GetUserApproval(ctx, t.approver, t, req)
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// EnvGetTool so that read access to environment variables can be
// cached on a per-variable or global basis. EnvGet is read-only and
// only ever requests ApprovalActionRead.
func (t EnvGetTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*EnvGetReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	varName := req.Envvar
	if varName == "" {
		return DefaultApprovalRequest(t, arg)
	}

	varPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupEnv, am.ApprovalTargetEnvVar, varName)

	prompt := fmt.Sprintf("gptcli would like to read environment variable %q. Allow?", varName)

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
		{
			Key:      "vr",
			Label:    "Yes, and allow all future reads of this variable",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: varPolicyID,
			Actions:  []am.ApprovalAction{am.ApprovalActionRead},
		},
		{
			Key:      "vw",
			Label:    "Yes, and allow all future reads or writes of this variable",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: varPolicyID,
			Actions:  []am.ApprovalAction{am.ApprovalActionRead, am.ApprovalActionWrite},
		},
		{
			Key:   "n",
			Label: "No",
			Scope: am.ApprovalScopeDeny,
		},
	}

	return am.ApprovalRequest{
		Prompt:          prompt,
		RequiredActions: []am.ApprovalAction{am.ApprovalActionRead},
		Choices:         choices,
	}
}

func NewEnvGetTool(approver am.Approver) types.GptCliTool {
	t := &EnvGetTool{
		approver: approver,
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

	err := GetUserApproval(ctx, t.approver, t, req)
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

// BuildApprovalRequest implements ToolWithCustomApproval for
// EnvSetTool so that write access to environment variables can be
// cached on a per-variable or global basis. Writes imply the ability
// to read as well.
func (t EnvSetTool) BuildApprovalRequest(arg any) am.ApprovalRequest {
	req, ok := arg.(*EnvSetReq)
	if !ok || req == nil {
		return DefaultApprovalRequest(t, arg)
	}

	varName := req.Envvar
	if varName == "" {
		return DefaultApprovalRequest(t, arg)
	}

	varPolicyID := am.ApprovalPolicyID(am.ApprovalSubsysTools,
		am.ApprovalGroupEnv, am.ApprovalTargetEnvVar, varName)

	prompt := fmt.Sprintf("gptcli would like to set environment variable %q. Allow?", varName)

	choices := []am.ApprovalChoice{
		{
			Key:   "y",
			Label: "Yes, this time only",
			Scope: am.ApprovalScopeOnce,
		},
		{
			Key:      "vw",
			Label:    "Yes, and allow all future reads or writes to this variable",
			Scope:    am.ApprovalScopeTarget,
			PolicyID: varPolicyID,
			Actions: []am.ApprovalAction{am.ApprovalActionWrite,
				am.ApprovalActionRead},
		},
		{
			Key:   "n",
			Label: "No",
			Scope: am.ApprovalScopeDeny,
		},
	}

	return am.ApprovalRequest{
		Prompt:          prompt,
		RequiredActions: []am.ApprovalAction{am.ApprovalActionWrite, am.ApprovalActionRead},
		Choices:         choices,
	}
}

func NewEnvSetTool(approver am.Approver) types.GptCliTool {
	t := &EnvSetTool{
		approver: approver,
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

	err := GetUserApproval(ctx, t.approver, t, req)
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
