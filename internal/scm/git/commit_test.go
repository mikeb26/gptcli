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

func TestCommit_ReturnsUntrackedFilesErrorWhenMissingInOptions(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_UNTRACKED":    "a.txt\nb.txt\n",
		"MOCK_GIT_STAGED_FILES": "staged.txt\n",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	untracked, err := c.Commit(context.Background(), "/tmp/repo", scm.CommitOptions{IncludeUntracked: map[string]bool{"a.txt": true}})
	if err == nil || !errors.Is(err, scm.ErrUntrackedFiles) {
		t.Fatalf("expected scm.ErrUntrackedFiles, got %v", err)
	}
	if untracked == nil || len(untracked.Filename) != 2 {
		t.Fatalf("expected untracked list, got %+v", untracked)
	}
}

func TestCommit_StagesTrackedAndSelectedUntracked(t *testing.T) {
	logPath, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_UNTRACKED":    "a.txt\nb.txt\n",
		"MOCK_GIT_STAGED_FILES": "tracked.go\na.txt\n",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	_, err := c.Commit(context.Background(), "/tmp/repo", scm.CommitOptions{IncludeUntracked: map[string]bool{"a.txt": true, "b.txt": false}})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	logs := readMockGitLog(t, logPath)
	joined := strings.Join(logs, "\n")
	if !strings.Contains(joined, "add -u") {
		t.Fatalf("expected git add -u in logs: %#v", logs)
	}
	if !strings.Contains(joined, "add -- a.txt") {
		t.Fatalf("expected git add -- a.txt in logs: %#v", logs)
	}
	if strings.Contains(joined, "add -- b.txt") {
		t.Fatalf("did not expect b.txt to be added: %#v", logs)
	}
	if !strings.Contains(joined, "diff --cached --name-only") {
		t.Fatalf("expected staged check in logs: %#v", logs)
	}
	if !strings.Contains(joined, "commit") {
		t.Fatalf("expected commit in logs: %#v", logs)
	}
}

func TestCommit_ReturnsNothingToCommit(t *testing.T) {
	_, cleanup := setupMockGit(t, map[string]string{
		"MOCK_GIT_UNTRACKED":    "",
		"MOCK_GIT_STAGED_FILES": "\n",
	})
	t.Cleanup(cleanup)

	c := NewClient()
	_, err := c.Commit(context.Background(), "/tmp/repo", scm.CommitOptions{IncludeUntracked: map[string]bool{}})
	if err == nil || !errors.Is(err, scm.ErrNothingToCommit) {
		t.Fatalf("expected scm.ErrNothingToCommit, got %v", err)
	}
}
