/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

// Package scm defines a small, VCS-agnostic abstraction for source control
// operations
package scm

import "context"

// CommitOptions controls the behavior of Client.Commit.
type CommitOptions struct {
	// IncludeUntracked lists all untracked files currently present in the repo
	// and whether each should be included in the commit.
	//
	// For every untracked file present in the repo at commit time, this map must
	// contain a key for that file:
	//   - true  => stage/include the file in the commit
	//   - false => do not stage/include the file in the commit
	//
	// If any untracked files are present and not mentioned in this map, Commit
	// will return an UntrackedFilesError so callers can prompt the user and
	// retry.
	IncludeUntracked map[string]bool
}

// UntrackedFiles indicates that set of untracked files that are present and
// which should be accounted for within CommitOptions.IncludeUntracked in order
// for a commit to proceed successfully
type UntrackedFiles struct {
	Filename []string
}

// DiffScope describes which set of changes should be presented by DiffTool.
type DiffScope int

const (
	// DiffScopeUncommitted shows all uncommitted changes (both staged and
	// unstaged).
	DiffScopeUncommitted DiffScope = iota
)

// Client is a VCS-agnostic client for the small set of source-control
// operations needed by the UI.
type Client interface {
	RepoStatusString(ctx context.Context, dir string) (string, error)
	DiffTool(ctx context.Context, dir string, scope DiffScope) error
	Commit(ctx context.Context, dir string, opts CommitOptions) (*UntrackedFiles, error)
}
