/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mikeb26/gptcli/internal/scm"
)

// DiffTool invokes `git difftool`.
//
// This is intended for interactive use (it wires stdio through), so it does not
// apply Client.Timeout when ctx has no deadline.

func (c *Client) DiffTool(ctx context.Context, dir string, scope scm.DiffScope) error {
	toolName, toolKey, err := c.determineDiffToolName(ctx, dir)
	if err != nil {
		return err
	}
	if toolName == "" {
		return ErrDiffToolUnconfigured
	}

	gitArgs := []string{"difftool"}
	// If the user has configured a GUI-specific difftool, prefer it.
	if toolKey == "diff.guitool" || toolKey == "merge.guitool" {
		gitArgs = append(gitArgs, "--gui")
	}
	switch scope {
	case scm.DiffScopeUncommitted:
		// Show both staged and unstaged changes. git-difftool doesn't have a
		// single flag for this, so invoke it twice.
		if err := c.runGitDiffTool(ctx, dir, append(gitArgs, "--cached", "--no-prompt")...); err != nil {
			return err
		}
		if err := c.runGitDiffTool(ctx, dir, append(gitArgs, "--no-prompt")...); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported diff scope: %d", scope)
	}
	return nil
}

func (c *Client) runGitDiffTool(ctx context.Context, dir string, gitArgs ...string) error {
	cmd := exec.CommandContext(ctx, "git", buildGitArgs(dir, gitArgs...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToExecuteGit, err)
	}
	return nil
}

func (c *Client) determineDiffToolName(ctx context.Context, dir string) (toolName string, toolKey string, err error) {
	// The selection order here mirrors git-difftool's documented behavior when
	// invoked with --gui.
	keys := []string{
		"diff.guitool",
		"merge.guitool",
		"diff.tool",
		"merge.tool",
	}

	for _, key := range keys {
		v, ok, err := c.gitConfigGet(ctx, dir, key)
		if err != nil {
			return "", "", err
		}
		v = strings.TrimSpace(v)
		if ok && v != "" {
			return v, key, nil
		}
	}

	return "", "", nil
}

func (c *Client) gitConfigGet(ctx context.Context, dir string, key string) (string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cmd := exec.CommandContext(ctx, "git", buildGitArgs(dir, "config", "--get", key)...)
	out, err := cmd.Output()
	if err == nil {
		v := strings.TrimSpace(string(out))
		if v == "" {
			return "", false, nil
		}
		return v, true, nil
	}

	// Exit code 1 indicates the key was not found.
	if ee, ok := err.(*exec.ExitError); ok {
		if ee.ExitCode() == 1 {
			return "", false, nil
		}
	}
	return "", false, fmt.Errorf("%w: %w", ErrFailedToExecuteGit, err)
}
