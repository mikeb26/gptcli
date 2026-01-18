/* Copyright © 2026 Mike Brown. All Rights Reserved.
 *
 * See LICENSE file at the root of this package for license terms
 */

package git

import (
	"errors"
)

var (
	ErrFailedToExecuteGit      = errors.New("failed to execute git")
	ErrFailedToDetermineBranch = errors.New("failed to determine current git branch")
	ErrDiffToolUnconfigured    = errors.New("no difftool configured; please run git config --global diff.tool to configure")
	ErrNotGitRepo              = errors.New("not a git repo")
)
