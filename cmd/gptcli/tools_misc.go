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

type PwdTool struct {
	input *bufio.Reader
}

type PwdReq struct {
}

type PwdResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the pwd call"`
	Pwd   string `json:"error" jsonschema:"description=The present working directory"`
}

type ChdirTool struct {
	input *bufio.Reader
}

type ChdirReq struct {
	Newdir string `json:"error" jsonschema:"description=The new directory to change into"`
}

type ChdirResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the chdir call"`
}

type EnvGetTool struct {
	input *bufio.Reader
}

type EnvGetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to get"`
}

type EnvGetResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the envget call"`
	Value string `json:"error" jsonschema:"description=The current value of the request environment variable"`
}

type EnvSetTool struct {
	input *bufio.Reader
}

type EnvSetReq struct {
	Envvar string `json:"envvar" jsonschema:"description=The environment variable to set"`
	Value  string `json:"error" jsonschema:"description=The new value to set"`
}

type EnvSetResp struct {
	Error string `json:"error" jsonschema:"description=The error status of the envset call"`
}

func (t PwdTool) GetOp() ToolCallOp {
	return Pwd
}

func (t PwdTool) RequiresUserApproval() bool {
	return false
}

func NewPwdTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &PwdTool{
		input: inputIn,
	}

	return t.Define()
}

func (t PwdTool) Define() internal.GptCliTool {
	ret, err := utils.InferTool(string(t.GetOp()), "print the current working directory",
		t.Invoke)
	if err != nil {
		panic(err)
	}

	return ret
}

func (t PwdTool) Invoke(ctx context.Context, _ *PwdReq) (*PwdResp, error) {
	ret := &PwdResp{}

	err := getUserApproval(t.input, t, "")
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

func (t ChdirTool) GetOp() ToolCallOp {
	return Chdir
}

func (t ChdirTool) RequiresUserApproval() bool {
	return true
}

func NewChdirTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &ChdirTool{
		input: inputIn,
	}

	return t.Define()
}

func (t ChdirTool) Define() internal.GptCliTool {
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

	err := getUserApproval(t.input, t, req)
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

func (t EnvGetTool) GetOp() ToolCallOp {
	return EnvGet
}

func (t EnvGetTool) RequiresUserApproval() bool {
	return true
}

func NewEnvGetTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &EnvGetTool{
		input: inputIn,
	}

	return t.Define()
}

func (t EnvGetTool) Define() internal.GptCliTool {
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

	err := getUserApproval(t.input, t, req)
	if err != nil {
		ret.Error = err.Error()
		return ret, nil
	}

	ret.Value = os.Getenv(req.Envvar)

	return ret, nil
}

func (t EnvSetTool) GetOp() ToolCallOp {
	return EnvSet
}

func (t EnvSetTool) RequiresUserApproval() bool {
	return true
}

func NewEnvSetTool(inputIn *bufio.Reader) internal.GptCliTool {
	t := &EnvSetTool{
		input: inputIn,
	}

	return t.Define()
}

func (t EnvSetTool) Define() internal.GptCliTool {
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

	err := getUserApproval(t.input, t, req)
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
