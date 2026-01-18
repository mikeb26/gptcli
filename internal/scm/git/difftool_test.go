/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mikeb26/gptcli/internal/scm"
)

func TestDetermineDiffToolName_PrefersGuiKeysFirst(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_CONFIG_GET_diff_guitool":  "meld",
		"MOCK_GIT_CONFIG_GET_merge_guitool": "kdiff3",
		"MOCK_GIT_CONFIG_GET_diff_tool":     "diffmerge",
		"MOCK_GIT_CONFIG_GET_merge_tool":    "opendiff",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	name, key, err := c.determineDiffToolName(context.Background(), "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if name != "meld" || key != "diff.guitool" {
		t.Fatalf("got (%q,%q)", name, key)
	}
}

func TestDiffTool_Unconfigured(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{})
	t.Cleanup(cleanup)
	c := NewClient()
	err := c.DiffTool(context.Background(), "/tmp/repo", scm.DiffScopeUncommitted)
	if err == nil || !errors.Is(err, ErrDiffToolUnconfigured) {
		t.Fatalf("expected ErrDiffToolUnconfigured, got %v", err)
	}
}

func TestDiffTool_UncommittedInvokesTwice(t *testing.T) {
	logPath, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_CONFIG_GET_diff_tool": "meld",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	err := c.DiffTool(context.Background(), "/tmp/repo", scm.DiffScopeUncommitted)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	logs := readMockGitLog(t, logPath)
	joined := strings.Join(logs, "\n")
	// Expect two difftool invocations: cached and working tree.
	if strings.Count(joined, "difftool") != 2 {
		t.Fatalf("expected 2 difftool invocations, logs: %#v", logs)
	}
	if !strings.Contains(joined, "difftool --cached --no-prompt") {
		t.Fatalf("expected cached difftool invocation, logs: %#v", logs)
	}
	if !strings.Contains(joined, "difftool --no-prompt") {
		t.Fatalf("expected non-cached difftool invocation, logs: %#v", logs)
	}
}

func TestGitConfigGet_NotFoundReturnsFalse(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{})
	t.Cleanup(cleanup)
	c := NewClient()
	_, ok, err := c.gitConfigGet(nil, "/tmp/repo", "diff.tool")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false")
	}
}
