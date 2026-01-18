/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildGitArgs(t *testing.T) {
	t.Parallel()

	if got := buildGitArgs("", "status"); len(got) != 1 || got[0] != "status" {
		t.Fatalf("unexpected args for empty dir: %#v", got)
	}

	got := buildGitArgs("/tmp/repo", "status", "--porcelain")
	if len(got) < 3 || got[0] != "-C" || got[1] != "/tmp/repo" || got[2] != "status" {
		t.Fatalf("unexpected args: %#v", got)
	}
}

func TestUpstreamDivergence_NonVerbose(t *testing.T) {
	t.Parallel()

	meta := porcelainMeta{upstream: "origin/main"}
	// Note: RepoStatusString only calls upstreamDivergence when env.ShowUpstream
	// is non-empty. upstreamDivergence itself does not special-case an empty
	// showUpstream; it treats it as non-verbose.
	if got := upstreamDivergence("verbose", porcelainMeta{}); got != "" {
		t.Fatalf("expected empty when no upstream, got %q", got)
	}

	meta = porcelainMeta{upstream: "origin/main", ahead: 0, behind: 0}
	if got := upstreamDivergence("auto", meta); got != "=" {
		t.Fatalf("got %q", got)
	}
	meta = porcelainMeta{upstream: "origin/main", ahead: 2, behind: 0}
	if got := upstreamDivergence("", meta); got != ">" {
		t.Fatalf("got %q", got)
	}
	meta = porcelainMeta{upstream: "origin/main", ahead: 0, behind: 1}
	if got := upstreamDivergence("name", meta); got != "<" {
		t.Fatalf("got %q", got)
	}
	meta = porcelainMeta{upstream: "origin/main", ahead: 2, behind: 3}
	if got := upstreamDivergence("auto", meta); got != "<>" {
		t.Fatalf("got %q", got)
	}
}

func TestUpstreamDivergence_VerboseName(t *testing.T) {
	t.Parallel()

	meta := porcelainMeta{upstream: "origin/main", ahead: 2, behind: 3}
	if got := upstreamDivergence("verbose name", meta); got != "|u+2-3 origin/main" {
		t.Fatalf("got %q", got)
	}
}

func TestRepoStatusString_NotGitRepo(t *testing.T) {
	logPath, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_INSIDE": "false",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	_, err := c.RepoStatusString(context.Background(), "/tmp/repo")
	if err == nil || !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("expected ErrNotGitRepo, got %v", err)
	}

	logs := readMockGitLog(t, logPath)
	if len(logs) == 0 || !strings.HasPrefix(logs[0], "rev-parse") {
		t.Fatalf("expected rev-parse call, logs: %#v", logs)
	}
}

func TestRepoStatusString_HideIfIgnoredTreatsAsNotRepo(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_INSIDE":       "true",
		"MOCK_GIT_CHECK_IGNORE": "1",
	})
	t.Cleanup(cleanup)
	// Force HideIfPwdIgnored to true.
	t.Setenv("GIT_PS1_HIDE_IF_PWD_IGNORED", "1")

	c := NewClient()
	_, err := c.RepoStatusString(context.Background(), "/tmp/repo")
	if err == nil || !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("expected ErrNotGitRepo, got %v", err)
	}
}

func TestRepoStatusString_StatusFailureFallsBackToBranch(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_INSIDE":            "true",
		"MOCK_GIT_TOPLEVEL":          "/tmp/repo",
		"MOCK_GIT_BRANCH":            "main",
		"MOCK_GIT_STATUS_EXIT":       "1",
		"MOCK_GIT_SYMBOLIC_REF_EXIT": "0",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	got, err := c.RepoStatusString(context.Background(), "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "repo:main" {
		t.Fatalf("got %q", got)
	}
}

func TestRepoStatusString_ComposesState(t *testing.T) {
	gitDir := t.TempDir()
	// Trigger REBASE in progress.
	if err := os.MkdirAll(filepath.Join(gitDir, "rebase-merge"), 0o755); err != nil {
		t.Fatalf("mkdir rebase-merge: %v", err)
	}
	// Trigger CONFLICT state.
	statusOut := "" +
		"# branch.upstream origin/main\n" +
		"# branch.ab +1 -2\n" +
		"1 .M N... 100644 100644 100644 abc def file.txt\n" +
		"? untracked.txt\n"

	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_INSIDE":                          "true",
		"MOCK_GIT_TOPLEVEL":                        "/tmp/repo",
		"MOCK_GIT_BRANCH":                          "main",
		"MOCK_GIT_STATUS_OUT":                      statusOut,
		"MOCK_GIT_HAS_STASH":                       "1",
		"MOCK_GIT_ABS_GIT_DIR":                     gitDir,
		"MOCK_GIT_UNMERGED":                        "some conflict\n",
		"MOCK_GIT_CONFIG_BOOL_core_sparseCheckout": "true",
	})
	t.Cleanup(cleanup)

	// Ensure defaults are enabled.
	t.Setenv("GIT_PS1_SHOWDIRTYSTATE", "1")
	t.Setenv("GIT_PS1_SHOWSTASHSTATE", "1")
	t.Setenv("GIT_PS1_SHOWUNTRACKEDFILES", "1")
	t.Setenv("GIT_PS1_SHOWUPSTREAM", "verbose name")
	t.Setenv("GIT_PS1_COMPRESSSPARSESTATE", "1")
	t.Setenv("GIT_PS1_SHOWCONFLICTSTATE", "yes")
	t.Setenv("GIT_PS1_STATESEPARATOR", " ")

	c := NewClient()
	got, err := c.RepoStatusString(context.Background(), "/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	want := "repo:main *$%?|REBASE|u+1-2 origin/main|CONFLICT"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRepoStatusString_TimeoutWhenNoDeadline(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_SLEEP_SECS": "0.25",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	c.Timeout = 20 * time.Millisecond
	_, err := c.RepoStatusString(context.Background(), "/tmp/repo")
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
