/* Copyright © 2025 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */
package types

type ToolCallOp string

const (
	RunCommand  ToolCallOp = "cmd_run"
	CreateFile             = "file_create"
	AppendFile             = "file_append"
	ReadFile               = "file_read"
	DeleteFile             = "file_delete"
	FilePatch              = "file_patch"
	Pwd                    = "dir_pwd"
	Chdir                  = "dir_chdir"
	EnvGet                 = "env_get"
	EnvSet                 = "env_set"
	RetrieveUrl            = "url_retrieve"
	RenderUrl              = "url_render"
	PromptRun              = "prompt_run"
)

type Tool interface {
	GetOp() ToolCallOp
	RequiresUserApproval() bool
	Define() GptCliTool
}
